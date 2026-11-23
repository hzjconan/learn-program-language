from p11_docs.processor import clean_records
import pytest

def test_clean_records_empty_input():
  records, errors = clean_records([])
  assert len(records) == 0
  assert len(errors) == 0

def test_clean_records_strips_and_deduplicates():
  records, errors = clean_records([
    (1, {"id": 1, "text": "  Hello  "}),
    (2, {"id": 2, "text": "Hello"}),
    (4, {"id": 3, "text": "hello"}),
    (5, {"id": 4, "text": "   "}),
    (6, {"id": 5, "text": 123}),
    (7, {"id": 6, "text": "  再见  "}),
  ])
  assert records == [
    {"id": 1, "text": "Hello"},
    {"id": 3, "text": "hello"},
    {"id": 6, "text": "再见"},
  ]

def test_clean_records_does_not_mutate_input():
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

@pytest.mark.parametrize("bad_record", [
    # 非对象：None / 列表 / 字符串 / 整数
    None,
    [],
    [1, 2],
    "not a dict",
    42,
    # 对象缺少 text
    {"id": 8},
    # text 非字符串：None / 整数 / 列表 / 布尔
    {"id": 9, "text": None},
    {"id": 10, "text": 123},
    {"id": 11, "text": [1, 2]},
    {"id": 12, "text": True},
    # text 为空串或只含空白
    {"id": 13, "text": ""},
    {"id": 14, "text": "   "},
    {"id": 15, "text": "\t\n"},
])
def test_clean_records_rejects_invalid_record(bad_record):
    records, errors = clean_records([
        (10, bad_record),
        (20, {"id": "ok", "text": "  OK  "}),
    ])
    # 1. 完整输出只有清洗后的好记录
    assert records == [{"id": "ok", "text": "OK"}]
    # 2. 错误恰好一条，行号为 10
    assert len(errors) == 1
    assert errors[0][0] == 10
    # 3. 错误消息是非空字符串
    assert isinstance(errors[0][1], str)
    assert len(errors[0][1]) > 0