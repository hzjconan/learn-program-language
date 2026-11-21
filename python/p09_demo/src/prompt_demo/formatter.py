from .settings import MAX_CHARS

def format_prompt(prompt):
    return prompt[:MAX_CHARS]