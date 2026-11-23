import encodings
import json

import pytest
from p11_docs.files import read_records, write_records

@pytest.fixture
def sample_jsonl(tmp_path):
    file = tmp_path / "greeting.jsonl"
    file.write_text('{"id": 1, "text": "  你好  "}\n\n坏数据\nnull\n{"id": 2, "text": "再见"}', encoding="utf-8")
    return file


def test_read_records_preserves_line_numbers(sample_jsonl):
    records, errors = read_records(sample_jsonl)

    # 完整的带行号记录列表：读取层不清洗文本，null 也是合法 JSON
    assert records == [
        (1, {"id": 1, "text": "  你好  "}),
        (4, None),
        (5, {"id": 2, "text": "再见"}),
    ]

    # 只有一项解析错误，真实行号为 3，消息是非空字符串
    assert len(errors) == 1
    assert errors[0][0] == 3
    assert isinstance(errors[0][1], str)
    assert len(errors[0][1]) > 0


def test_read_records_missing_file_raises(tmp_path):
    missing = tmp_path / "does_not_exist.jsonl"
    with pytest.raises(FileNotFoundError):
        read_records(missing)


def test_write_records_round_trip(tmp_path):
    records = [
        {"id": 1, "text": "你好"},
        {"id": 2, "text": "你好\n世界"},
        {"id": 3, "text": "line1\nline2"},
    ]
    target = tmp_path / "out.jsonl"
    write_records(target, records)

    # 用 UTF-8 读取，中文直接可见（不调用项目的 read_records）
    content = target.read_text(encoding="utf-8")
    assert "你好" in content
    assert "世界" in content

    # 文件物理行数等于记录数，末尾有换行；
    # 值内换行被 JSON 编码成 \\n，不产生额外物理行
    assert content.endswith("\n")
    assert content.count("\n") == len(records)

    # 每行 json.loads 解析后，与原记录列表完整相等
    parsed = [json.loads(line) for line in content.splitlines()]
    assert parsed == records


def test_write_records_does_not_overwrite(tmp_path):
    target = tmp_path / "existing.jsonl"
    original = b"original content\n"
    target.write_bytes(original)

    before = target.read_bytes()

    with pytest.raises(FileExistsError):
        write_records(target, [{"id": 1, "text": "x"}])

    # 断言放在 with 外面：异常发生后仍会执行，证明原文件没被破坏
    after = target.read_bytes()
    assert after == before
    assert after == original