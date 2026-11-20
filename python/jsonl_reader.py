import json
from pathlib import Path
from tempfile import TemporaryDirectory

def load_jsonl(path):
    records = []
    erros = []
    with Path(path).open("r", encoding="utf-8") as f:
        for line_num, line in enumerate(f, start=1):
            if len(line.strip()) > 0:
                try:
                    records.append(json.loads(line))
                except json.JSONDecodeError as exc:
                    erros.append((line_num, exc.msg))
    return (records, erros)

def main():
    with TemporaryDirectory() as directory:
        path = Path(directory) / "prompts.jsonl"
        path.write_text('{"id": 1, "prompt": "你好"}\n\n坏数据\n{"id": 2, "prompt": "再见"}', encoding="utf-8")

        records, erros = load_jsonl(path)
        assert len(records) == 2
        assert records[0]["id"] == 1
        assert records[1]["id"] == 2
        assert erros[0][0] == 3

if __name__ == "__main__":
    main()