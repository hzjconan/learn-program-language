"""A03 工具注册练习：用户实现，已通过正常路径与错误边界验证。"""

import inspect


TOOLS = {}


def register(fn):
    """按函数名字注册；重复名字抛出 ValueError；保留原函数。"""
    if fn.__name__ in TOOLS:
        raise ValueError(f"工具 {fn.__name__} 已注册")
    TOOLS[fn.__name__] = fn
    return fn


def describe_tool(name):
    """返回工具信息；名字不存在时允许 KeyError 向上传播。"""
    fn = TOOLS[name]
    return {
        "name": name,
        "description": fn.__doc__,
        "signature": str(inspect.signature(fn)),
    }


@register
def search(query: str, *, limit: int = 5) -> list[str]:
    """搜索文档。"""
    return [query] * limit


if __name__ == "__main__":
    print(describe_tool("search"))
    print(TOOLS["search"]("python", limit=2))
    print(TOOLS["search"] is search)
