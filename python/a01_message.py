from typing import TypedDict, Literal

class Message(TypedDict):
    role: Literal["user", "assistant"]
    content: str

def make_message(role: Literal["user", "assistant"], content: str) -> Message:
    return {"role": role, "content": content}

def find_last_user_text(messages: list[Message]) -> str | None:
    last_message = next((msg for msg in reversed(messages) if msg["role"] == "user"), None)
    return last_message["content"] if last_message else None


if __name__ == "__main__":
    user_message = make_message("user", "你好")
    assert user_message == {"role": "user", "content": "你好"}
    assistant_message = make_message("assistant", "你好，我是助手")
    assert assistant_message == {"role": "assistant", "content": "你好，我是助手"}

    assert find_last_user_text([]) is None
    assert find_last_user_text([{"role": "assistant", "content": "你好，我是助手"}]) is None
    assert find_last_user_text([
        {"role": "user", "content": "你好"},
        {"role": "assistant", "content": "你好，我是助手"},
        {"role": "user", "content": "明天天气如何"},
        {"role": "assistant", "content": "晴天"},
        ]
    ) == "明天天气如何"

    assert find_last_user_text([
        {"role": "user", "content": "你好"},
        {"role": "assistant", "content": "你好，我是助手"},
        {"role": "user", "content": ""},
        {"role": "assistant", "content": "请再说一遍"},
        ]
    ) == ""
