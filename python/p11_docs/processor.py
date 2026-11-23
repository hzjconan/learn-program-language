from p10_pipeline import StripCleaner, Pipeline

def clean_records(numbered_records):
    text_seen = set()
    records = []
    errors = []
    p = Pipeline([StripCleaner()])
    for line_no, record in numbered_records:
        if not isinstance(record, dict):
            errors.append((line_no, "记录必须是 JSON 对象"))
            continue
        if "text" not in record:
            errors.append((line_no, "记录必须包含 text 字段"))
            continue
        if type(record["text"]) is not str or len(record["text"].strip()) == 0:
            errors.append((line_no, f"text {record["text"]} in Record {line_no} is not valid"))
            continue
        processed_text = p.process(record["text"])
        if processed_text not in text_seen:
            text_seen.add(processed_text)
            records.append({**record, "text": processed_text})

    return (records, errors)

def main():
    source = [
    (1, {"id": 1, "text": "  Hello  "}),
    (2, {"id": 2, "text": "Hello"}),
    (4, {"id": 3, "text": "hello"}),
    (5, {"id": 4, "text": "   "}),
    (6, {"id": 5, "text": 123}),
    (7, {"id": 6, "text": "  再见  "}),
]
    records, errors = clean_records(source)

    assert records == [
        {"id": 1, "text": "Hello"},
        {"id": 3, "text": "hello"},
        {"id": 6, "text": "再见"},
    ]
    assert [line_no for line_no, message in errors] == [5, 6]
    assert source[0][1]["text"] == "  Hello  "
    assert records[0] is not source[0][1]

    records, errors = clean_records([
    (10, None),
    (11, {"id": 8}),
    (12, {"text": " OK "}),
])

    assert records == [{"text": "OK"}]
    assert [line_no for line_no, message in errors] == [10, 11]

if __name__ == "__main__":
    main()
