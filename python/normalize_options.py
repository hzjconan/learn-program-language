def normalize_options(temperature, max_tokens, docs):
    temperature = 0.7 if temperature is None else temperature
    if not (0 <= temperature <= 2):
        raise ValueError("temperature 必须在 0 和 2 之间，包含边界")

    max_tokens = 1024 if max_tokens is None else max_tokens
    if max_tokens <= 0:
        raise ValueError("max_tokens 必须大于 0")

    docs = ["默认文档"] if docs is None else docs

    return dict(temperature=temperature, max_tokens=max_tokens, docs=docs)

if __name__ == "__main__":
    print(normalize_options(None, None, None))
    print(normalize_options(0, 1, []))
    print(normalize_options(2, 100, ["文档A"]))
    print(normalize_options(-0.1, 100, []))  # 应报错
    normalize_options(1, 0, [])      # 应报错
    normalize_options(2.1, 100, [])  # 应报错