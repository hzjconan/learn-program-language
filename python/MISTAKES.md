# 错题本

只记录实际答错、理解不完整或评审中暴露的问题，不预设用户会犯哪些错。

每条记录：原题与原答案、问题所在、正确机制、验证方式、换场景复测记录。再次答对不删除历史。

## M01 · 虚拟环境判断依赖目录名（P01）

原实现：`'.venv' in sys.prefix`。环境改名即误判。标准 venv 判断比较 sys.prefix 与 sys.base_prefix，base_prefix 是基础安装前缀，不是解释器文件。用户已修正并解释改名变式，验证通过。见 [P01](notes/P01.md)。

## M02 · 值相等与对象身份混淆（P02）

原题 a、b 分别为 [1, 2]，c=a；用户原答 False True True。正确 True False True：list 的 == 按内容比较，is 比较对象身份。后续变式输出正确，解释“不同变量”仍需修正为“分别创建且不共享对象”。P04 继续回访。见 [P02](notes/P02.md)。

## M03 · 参数边界与默认类型（P02）

max_tokens 要求 >0，初版 <0 漏拒零；docs 默认写字符串而非列表。用户改为 <=0 与 ["默认文档"]，教师分别验证有效/非法输入通过。复测时更换上下界场景。见 [P02](notes/P02.md)。

## M04 · get 的默认值不是 None 回退（P03）

原答 source 为 None 返回“未知”。正确是键存在就返回 None；default 仅键不存在时生效。随后 config limit=None 变式答对。见 [P03](notes/P03.md)。

## M05 · 原地排序的返回值（P03）

原答 scores 与 result 都是排序后列表。list.sort 修改列表、返回 None；sorted 返回新列表。随后 sorted 不改原列表变式及主练习通过。见 [P03](notes/P03.md)。

## M06 · 推导式边界与名字（P03）

原写 >0.8 而要求 >=0.8，另将 doc 写成 doce。前者漏边界，后者 NameError。教师已说明；后续 prepare_results 正确使用 >=。见 [P03](notes/P03.md)。

## M07 · zip 配对结果与字典混淆（P03）

原答 list(zip(ids,scores)) 为字典；实际为元组列表，默认截断到最短输入。strict=True 检查消费时长度，不检查键唯一性；用户在重复 ID 变式中正确区分列表与字典覆盖，验收通过。见 [P03](notes/P03.md)。
