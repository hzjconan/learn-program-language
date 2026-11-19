def make_prompt_filter(max_chars):
    def filter(prompt):
        return len(prompt) <= max_chars
    return filter

def make_prompt_filters(limits):
    filters = []
    for limit in limits:
        filters.append(make_prompt_filter(limit))
    return filters

def main():
    short, long = make_prompt_filters([3, 5])
    assert short("hey")
    assert not short("hello")
    assert long("hello")
    assert not long("hello!")

if __name__ == "__main__":
    main()