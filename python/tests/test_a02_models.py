from a02_models import parse_ticket
from pydantic import ValidationError
import pytest

def test_parse_ticket_correct():
    raw = '{"title": "天气", "priority": "low", "tags": ["a", "b"]}'
    ticket = parse_ticket(raw)
    assert ticket.title == "天气"
    assert ticket.priority == "low"
    assert ticket.tags == ["a", "b"]
    assert ticket.model_dump() == {"title": "天气", "priority": "low", "tags": ["a", "b"]}

def test_parse_ticket_separator_tags():
    raw1= '{"title": "天气", "priority": "low"}'
    ticket1 = parse_ticket(raw1)

    raw2= '{"title": "早餐", "priority": "high"}'
    ticket2 = parse_ticket(raw2)

    ticket1.tags.append("x")

    assert ticket2.tags == []
    assert ticket1.tags is not ticket2.tags

@pytest.mark.parametrize("raw, expected_loc", [
    ('{"title": "天气", "priority": "low"',              ()),          # JSON 解析错误，位于根
    ('{"priority": "low"}',                              ("title",)),  # 缺少必填字段
    ('{"title": "", "priority": "low"}',                 ("title",)),  # min_length=1 不满足
    ('{"title": 123, "priority": "low"}',                ("title",)),  # strict=True 拒绝 int→str 强制
    ('{"title": "天气", "priority": "lowest"}',          ("priority",)),  # Literal 不在可选值内
    ('{"title": "天气", "priority": "low", "tags": ["a", 123]}', ("tags", 1)),   # tags 中索引 1 的元素类型错
    ('{"title": "天气", "priority": "low", "tags": ["a", "b"], "debug": true}', ("debug",)),  # extra="forbid"
])
def test_parse_ticket_separator_invalid_input(raw: str, expected_loc: tuple):
    with pytest.raises(ValidationError) as exc_info:
        parse_ticket(raw)

    errors = exc_info.value.errors()
    locs = [e["loc"] for e in errors]
    assert expected_loc in locs, f"期望 loc {expected_loc!r}，实际 {locs!r}"