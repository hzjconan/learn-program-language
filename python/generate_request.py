def generate(prompt, /, *context, model="demo", temperature=0.7, max_tokens=100):
    return {
        "prompt": prompt,
        "context": context,
        "model": model,
        "temperature": temperature,
        "max_tokens": max_tokens,
    }

def generate_with_defaults(prompt, /, *context, **overrides):
    model = overrides.get("model", "demo")
    temperature = overrides.get("temperature", 0.7)
    max_tokens = overrides.get("max_tokens", 200)
    overrides = {**overrides, "model": model, "temperature": temperature, "max_tokens": max_tokens}
    return generate(prompt, *context, **overrides)

def main():
    result1 = generate_with_defaults("你好")
    assert result1 == {"prompt": "你好", "context": (), "model": "demo", "temperature": 0.7, "max_tokens": 200}

    result2 = generate_with_defaults("总结", "材料A", "材料B", temperature=0, max_tokens=50)
    assert result2 == {"prompt": "总结", "context": ("材料A", "材料B"), "model": "demo", "temperature": 0, "max_tokens": 50}

if __name__ == "__main__":
    main()