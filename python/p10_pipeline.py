class StripCleaner:
    def clean(self, text):
        return text.strip()

class LowerCleaner:
    def clean(self, text):
        return text.lower()

class Pipeline:
    def __init__(self, steps):
        self.history = []
        self.steps = [*steps]

    def process(self, text):
        for step in self.steps:
            text = step.clean(text)
        self.history.append(text)
        return text

    def __len__(self):
        return len(self.steps)

    def __call__(self, text: str) -> str:
        return self.process(text)

    def __repr__(self) -> str:
        return f"Pipeline(steps={len(self.steps)}, processed={len(self.history)})"

def main():
    steps = [StripCleaner(), LowerCleaner()]
    p = Pipeline(steps)
    q = Pipeline([])

    steps.clear()

    assert len(p) == 2
    assert p("  HELLO  ") == "hello"
    assert p.history == ["hello"]
    assert repr(p) == "Pipeline(steps=2, processed=1)"

    assert q.process("  Keep  ") == "  Keep  "
    assert q.history == ["  Keep  "]

if __name__ == "__main__":
    main()