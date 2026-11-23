import json
from .processor import clean_records
from tempfile import TemporaryDirectory
from pathlib import Path

def read_records(filename):
    numbered_records = []
    erros = []

    with open(filename, "r", encoding="utf-8") as f:
        for line_no, data in enumerate(f, start=1):
            try:
                if len(data.strip()) == 0:
                    continue
                numbered_records.append((line_no, json.loads(data)))
            except json.JSONDecodeError as e:
                erros.append((line_no, str(e)))
    return numbered_records, erros

def write_records(path, records):
    with open(path, "x", encoding="utf-8") as f:
        for record in records:
            f.write(json.dumps(record, ensure_ascii=False) + "\n")

def main():
    with TemporaryDirectory() as directory:
        sample = Path(directory) / "sample.jsonl"
        sample.write_text('{"id": 1, "text": "  你好  "}\n\n坏数据\nnull\n{"id": 2, "text": "你好"}', encoding="utf-8")

        records, parse_errors = read_records(sample)
        assert [r[0] for r in records] == [1, 4, 5]
        assert [e[0] for e in parse_errors] == [3]

        processed_records, validation_errors = clean_records(records)
        assert processed_records == [{"id": 1, "text": "你好"}]
        assert [line_no for line_no, _ in validation_errors] == [4]

        target = Path(directory) / "target.jsonl"
        write_records(target, processed_records)


        read_back, read_errors = read_records(target)
        assert read_errors == []
        assert [record for _, record in read_back] == processed_records

        hasFileExistsError = False
        try:
            write_records(target.absolute(), processed_records)
        except FileExistsError:
            hasFileExistsError = True
        assert hasFileExistsError == True

        hasFileNotFoundError = False
        try:
            read_records(str(target.absolute()) + ".bak")
        except FileNotFoundError:
            hasFileNotFoundError = True
        assert hasFileNotFoundError == True

if __name__ == "__main__":
    main()
