from a01_message import Message, make_message
from typing import List, Literal
from dataclasses import dataclass, field
from pydantic import Field, BaseModel, ConfigDict

@dataclass
class Conversation:
    title: str
    messages: list[Message] = field(default_factory=list)

def main():
    c1 = Conversation("天气")
    c2 = Conversation("早餐")
    c1.messages.append(make_message("user", "你好"))

    assert c1.messages[0]["content"] == "你好"
    assert len(c2.messages) == 0
    assert c1.messages is not c2.messages

class Ticket(BaseModel):
    model_config = ConfigDict(strict=True, extra="forbid")

    title: str = Field(min_length=1)
    priority: Literal["low", "normal", "high"]
    tags: list[str] = Field(default_factory=list)

def parse_ticket(raw: str) -> Ticket:
    return Ticket.model_validate_json(raw)

if __name__ == "__main__":
    main()
