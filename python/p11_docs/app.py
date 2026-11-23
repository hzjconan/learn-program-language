import argparse
import sys
from pathlib import Path
from .files import read_records, write_records
from .processor import clean_records

def main():
    parser = argparse.ArgumentParser(description="清洗并去重 JSONL 文档")
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    try:
        if Path.resolve(args.input) == Path.resolve(args.output):
            print(f"输入和输出文件不能相同: {args.input}", file=sys.stderr)
            return 1
        records, parse_errors = read_records(args.input)
        processed_records, validation_errors = clean_records(records)
        write_records(args.output, processed_records)

        errors = sorted([*parse_errors, *validation_errors], key=lambda e: e[0])
        for e in errors:
            print(f"line {e[0]}: {e[1]}", file=sys.stderr)

        print(f"written={len(processed_records)} errors={len(errors)}")
    except (OSError, UnicodeError) as e:
        print(f"文件处理失败: {e}", file=sys.stderr)
        return 1

    return 0
if __name__ == "__main__":
     sys.exit(main())
