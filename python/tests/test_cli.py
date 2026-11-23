import json
import subprocess
import sys
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent


def run_cli(*args):
    """在子进程中运行 p11_docs.app，返回 CompletedProcess。"""
    return subprocess.run(
        [sys.executable, "-m", "p11_docs.app", *map(str, args)],
        capture_output=True,
        text=True,
        cwd=str(PROJECT_ROOT),
        timeout=10,
    )


def _error_line_numbers(stderr):
    """从 stderr 解析 ``line N: ...`` 前缀，返回按出现顺序的行号列表。"""
    numbers = []
    for line in stderr.splitlines():
        if not line.strip():
            continue
        assert line.startswith("line "), repr(line)
        prefix = line.split(":", 1)[0]  # "line N"
        numbers.append(int(prefix.split()[1]))
    return numbers


def test_cli_processes_mixed_input(tmp_path):
    # 有效中文 / 清洗后重复 / 坏 JSON / 字段错误交错：
    #   行 2(坏JSON) 行 3(缺字段) 行 5(坏JSON) 行 6(text非字符串)
    # 若只拼接不排序，行号会变成 [2,5,3,6]，此测试能抓出该问题。
    source = tmp_path / "mixed.jsonl"
    source.write_text(
        "\n".join([
            '{"id": 1, "text": "  苹果  "}',  # 行 1：有效，清洗后 "苹果"
            "坏数据",                            # 行 2：坏 JSON
            '{"id": 3}',                        # 行 3：缺 text 字段
            '{"id": 4, "text": "苹果"}',        # 行 4：与行 1 清洗后重复，被去重
            "{{bad",                            # 行 5：坏 JSON
            '{"id": 6, "text": 123}',           # 行 6：text 非字符串
            '{"id": 7, "text": "  香蕉  "}',   # 行 7：有效，清洗后 "香蕉"
        ]) + "\n",
        encoding="utf-8",
    )
    target = tmp_path / "out.jsonl"

    result = run_cli("--input", source, "--output", target)

    # 退出码 0，stdout 是准确摘要
    assert result.returncode == 0
    assert result.stdout == "written=2 errors=4\n"

    # stderr 错误行号按顺序排列、数量正确，无 traceback
    assert "Traceback" not in result.stderr
    assert _error_line_numbers(result.stderr) == [2, 3, 5, 6]

    # 输出文件逐行解析后，与完整期望记录相等
    parsed = [json.loads(line) for line in target.read_text(encoding="utf-8").splitlines()]
    assert parsed == [
        {"id": 1, "text": "苹果"},
        {"id": 7, "text": "香蕉"},
    ]


def test_cli_missing_input(tmp_path):
    source = tmp_path / "missing.jsonl"  # 不创建：输入不存在
    target = tmp_path / "out.jsonl"

    result = run_cli("--input", source, "--output", target)

    assert result.returncode == 1
    assert result.stdout == ""
    assert result.stderr.strip()  # 有诊断
    assert "Traceback" not in result.stderr
    assert not target.exists()  # 输出未创建


def test_cli_does_not_overwrite(tmp_path):
    source = tmp_path / "in.jsonl"
    source.write_text('{"id": 1, "text": "  你好  "}\n', encoding="utf-8")

    target = tmp_path / "out.jsonl"
    original = b"pre-existing content\n"
    target.write_bytes(original)

    result = run_cli("--input", source, "--output", target)

    assert result.returncode == 1
    assert result.stdout == ""
    assert result.stderr.strip()  # 有诊断
    assert "Traceback" not in result.stderr
    assert target.read_bytes() == original  # 已有输出 bytes 不变
