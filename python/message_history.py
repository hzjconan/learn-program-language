import copy

def append_message(history, role, content):
    if history is None:
        history = []
    else:
        history = copy.deepcopy(history)
    history.append({"role": role, "content": content})
    return history

def main():
    original = [
            {
                "role": "user",
                "content": "你好",
                "metadata": {"tags": ["initial"]},
            }
        ]
    new_history = append_message(original, "assistant", "你好，我是一个助手")

    assert len(original) == 1
    assert len(new_history) == 2

    new_history[0]["content"] = "xxxxx"
    assert original[0]["content"] == "你好"

    new_history[0]["metadata"]["tags"].append("new")
    assert original[0]["metadata"]["tags"] == ["initial"]

    assert append_message(None, "role1", "content1") is not append_message(None, "role2", "content2")

if __name__ == "__main__":
    main()
