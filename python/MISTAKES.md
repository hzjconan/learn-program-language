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

## M08 · eval 模式不冻结参数（Q01）

原答：“model.eval()只是切换到评估模式，期间不会更新参数”。模式切换本身不禁止求导或更新；no_grad 不记录新的求导图，而正常评估不执行 optimizer.step。用户在最后变式中正确指出 eval 不阻止更新，验收通过。no_grad 也不清已有梯度。见 [Q01 答题记录](notes/Q01.md)。

## M09 · 预测输出时读错对象或忽略后续修改（P04）

浅拷贝题已解释新字典独立，却将原字典 score 写成 0（应 0.9）；深拷贝题把 tags 输出答成 score；默认列表题知道对象共享，却忽略第二次调用发生在 print(first) 前。预测时按执行顺序跟踪对象，再核对 print 读取的对象和字段。后续主练习与删除拷贝变式通过，未来读代码继续回访。见 [P04](notes/P04.md)。

## M10 · 显式传入零仍使用了参数默认值（P05）

预测 `request(temperature=0, model="demo")` 时使用了 `0.7`。关键字按名绑定，传入 `0` 就会保留。只有省略参数才使用默认值。练习已验证 `temperature=0` 能转发。见 [P05](notes/P05.md)。

## M11 · 字典覆盖与函数调用冲突混淆（P05）

预测两份字典在调用中展开时，以为可以收到两个同名 `temperature`。`{**defaults, **overrides}` 先合并字典，后者覆盖前者。`f(**defaults, **overrides)` 有重名键时会在进入函数前报 `TypeError`。封装时还需转发合并后的关键字，否则像 `max_token` 这样的拼写错误会被静默忽略。修订后复测通过。见 [P05](notes/P05.md)。

## M12 · 局部变量赋值前读取（P06）

预测 `choose_model()` 先打印模块变量，再赋值给局部变量。函数内的普通赋值使该名字在整个函数里被视为局部。在赋值前读取会触发 `UnboundLocalError`。见 [P06](notes/P06.md)。

## M13 · 外层绑定更新与闭包状态（P06）

`nonlocal model` 赋值后，外层再读也得到更新值。计数器题里，返回发生在自增之后，每次工厂调用都保留独立计数。用户随后答对别名变式：`alias = a` 与 `a` 共用同一个函数及其计数。见 [P06](notes/P06.md)。

## M14 · 文件末尾与行尾换行混淆（P08）

预测没有换行的末行输出 `'再见\n'`，认为读完后再调用 `readline()` 会报错。读取时保留文件原有行尾；打开的文件读到末尾时返回 `''`，空白行则读到 `'\n'`。见 [P08](notes/P08.md)。

## M15 · JSON 解析错误的两种行号（P08）

逐行解析时，`str(exc)` 包含传给 `json.loads` 的字符串内部行号。用 `enumerate(file, start=1)` 取实际文件行号，配合 `exc.msg` 记录错误原因。修订后第三行的错误返回 `(3, 'Expecting value')`。见 [P08](notes/P08.md)。

## M16 · 导入绑定与共享对象的修改（P09）

预测给 `settings.timeout` 重新赋值后，本地导入的 `timeout` 也会变化。`from settings import timeout` 在当前模块绑定导入当时取得的对象，模块属性后来可以指向别处。只要两个名字还指向同一个列表，`append` 修改的内容就会从两边看到。列表重新绑定后的变式已答对。见 [P09](notes/P09.md)。

## M17 · 直接运行文件时的包上下文（P09）

预测 `python demo/app.py` 也能执行 `from .settings import timeout`。直接运行这个脚本时缺少包上下文，首行相对导入会抛出 `ImportError`。从能找到包的目录执行 `python -m demo.app`，Python 就能解析 `.settings`。主练习通过模块入口和单独导入两项复测。见 [P09](notes/P09.md)。

## M18 · 把容器标注当作运行时检查（A01）

原答 `tags: list[str]` 后调用 append(42) 会报错。普通列表不会根据标注检查元素，可得到 ['python', 42]；静态检查工具应报告类型不符。后续正确预测返回标注不转换整数，以及不符合 Literal/TypedDict 的字典普通执行仍输出123，复测通过。见 [A01](notes/A01.md)。

## M19 · 分支语法与可选返回值（A01）

处理 str | None 时原写 else if，Python 应使用 elif；已排除 None 时可直接 else。用户修订为 is None / else，并正确说明空字符串进入 else。收尾换场景正确指出对字符串作真假判断会把空字符串错误变成 None。见 [A01](notes/A01.md)。

## M20 · 测试与实现共享字段拼写错误（A01）

Message 声明 content，但构造函数及两处断言都写 contebt，原断言通过。教师按需求独立断言 content 字段时失败；用户修订三处后通过。期望值应独立依据契约；TypedDict 不自动在运行时拒绝结构错误。见 [A01](notes/A01.md)。

## M21 · 混用数据类与校验模型的字段工具（A02）

原 Ticket 没有继承 BaseModel，Literal 使用调用括号，tags 使用 dataclasses.field，也未补 strict/extra 配置。Pydantic 模型需继承 BaseModel，Literal 用方括号，模型字段用 Pydantic Field；model_config 是 BaseModel 识别的固定配置属性名。用户自行修订后，类定义、默认列表隔离及7组非法字段独立验证通过。见 [A02](notes/A02.md)。

## M22 · 测试抛了预期异常，却没有触发目标规则（A02）

原7组坏输入中的前6组缺 JSON 右花括号，虽然 pytest.raises(ValidationError) 全部通过，实际都触发 json_invalid，没有验证缺字段、空文本或类型规则。修订为合法 JSON 的单一字段错误，并参数化 raw/expected_loc、断言异常 errors() 的字段路径后，9项测试通过。列表元素路径包含索引，如 ("tags", 1)。见 [A02](notes/A02.md)。

## M23 · 参数收集的类型与实际传入值（A03）

调用 total(10, 20, tax=0.1) 时，用户原答 args=[10, 20]、kwargs={tax: 0}。定义中的 *args 收集为 tuple，即 (10, 20)；**kwargs 收集为字符串键的 dict，即 {"tax": 0.1}，不会换成原函数的默认值。fn(args, kwargs) 是传两个容器，fn(*args, **kwargs) 才是解包转发。教师批改后，用户自行修正包装函数，注册练习与 limit 关键字专用参数预测通过；收集容器的类型与实际值仍在后续课程回访。见 [A03](notes/A03.md)。

## M24 · 原函数算出结果不等于包装函数返回结果（A03）

读错题只修正解包、未添加 return 时，用户原答输出33。原函数的计算结果会被丢弃，wrapper 正常执行到底返回 None。用户自行修正为 return fn(*args, **kwargs)，并准确解释外层 return wrapper 与内层返回结果的职责。换场景改为登记装饰器漏写 return fn，用户正确预测 None、6、直接调用报错，并说明名字绑定为 None，推理复测通过；主练习实际运行也验证了函数身份与返回值。见 [A03](notes/A03.md)。

## M25 · 耗尽的迭代器转列表返回空列表（A04）

原预测第二次 list(it) 返回 None，但已正确解释第一次收集使迭代器耗尽。list 会处理 StopIteration，并返回收集结果；零项就是 []，直接 next(it) 才会抛出结束信号。换场景用 alias = iter(it) 共享进度，用户准确预测耗尽后 list(alias) 得到 []，推理复测通过。主练习测试实际验证了耗尽后再次 list 返回 []。见 [A04](notes/A04.md)。

## M26 · yield 产出值与消费者打印结果混淆（A04）

原预测 print(list(g)) 的一段输出是20、end，漏掉恢复后的 between，也未把20写成列表。yield 20 只向 list 提供元素；list 会继续索取到耗尽，期间先打印 between 和 end，之后返回 [20]，外层 print 最后才打印列表。用户修订完整输出并正确解释 end 为何先出现；主练习按批产出与惰性读取验证通过。A05 两级生成器变式又漏写 print(next(stream)) 的外层 A 输出，教师指出内部 keep A 与外层 A 属于不同打印后，用户补齐完整输出并解释 list 收集片段的过程，修订通过；增量事件与中途失败预测也通过。见 [A04](notes/A04.md)、[A05](notes/A05.md)。

## M27 · break 退出循环不等于关闭生成器（A05）

原预测 open、piece A、close、inside、outside，并回答 close 在 break 时打印。break 只退出 for，仍需执行 with 内的 inside；离开 with 时 closing 才调用生成器close，执行finally打印close。用户修订输出为 open、piece A、inside、close、outside，并正确解释两层作用域。用户独立测试实际验证提前退出 closing 后来源关闭且没有预读；最终改为消费者抛 ValueError，用户正确预测 delta、source closed、caught，且不会产出后续片段或done，清理链复测通过。见 [A05](notes/A05.md)。

## M28 · 默认create_task提交调度不等于立即执行函数体（A06）

原预测start A、start B、end A、end B、created、a、b，并解释create_task后立即运行。Python 3.12默认任务工厂下先排队，main继续打印created，在等待未完成的A时才让出控制权；后续输出start A、start B、end A、end B、a、b。用户修订完整顺序并解释提交调度后当前main继续执行；主实现及交错执行测试通过。最终对已完成Task重复await的题目也正确指出只取结果、不重跑函数体、不因该等待让出。后续A07回访任务启动时机。见 [A06](notes/A06.md)。

## M29 · 调用参数先求值，空输入分支不能绕过未定义名字（A06）

原测试run_jobs([], demo_worker)，测试模块没有定义或导入demo_worker，实际NameError发生在调用run_jobs之前。用户改为测试本地的async worker，一旦执行则抛AssertionError，空输入返回[]且测试通过。另补测试覆盖缺口：原全部worker仅sleep(0)一次，完成顺序与输入相同，难以识别误按完成顺序返回；修订让A额外等待并断言B、C先完成，结果仍['a', 'b', 'c']，最终两项测试通过。见 [A06](notes/A06.md)。

## M30 · await不必然切换，暂停也不释放许可（A07）

原Semaphore题先写完wait A/B/C再enter A/B；可用许可可立即获取，A到sleep(0)才让出执行机会，且仍持有许可。用户追问“让出”是否让出sem，教师澄清执行机会与许可不同，用户后续修订任务顺序正确，gather结果列表格式亦已纠正。队列题再把ready放末尾、get A放put begin B前；main提交Task后先打印ready，put A有空位可直接完成，生产者继续打印put begin B，直到put B才等待。用户修订完整队列输出通过，实际编码运行符合预期。见 [A07](notes/A07.md)。

## M31 · 测试别名与只在发生异常时断言的缺口（A07）

原用names2=names后断言相等与同一对象，无法发现原地修改；try/except仅在抛ValueError时检查，未强制非法limit必须抛错，limit=0直接返回[]仍可通过。教师在独立进程换入错误接受0和原地反转输入的实现，确认测试均会放过，未修改用户文件。用户修订为copy快照、pytest.raises、恢复结果断言，补空输入非法上限和异常路径statistics()==(0, 0)，限并发7项通过。队列测试同样补齐空输入非法参数和执行即失败的worker，11项通过。见 [A07](notes/A07.md)。

## M32 · asyncio.Lock需异步上下文管理协议（A07）

用户圈定完整读改写范围正确，但写with lock；实际抛TypeError，Lock不支持同步上下文管理协议。async with使用异步进入/退出，获取锁可能等待；普通closing等同步管理器在async def内仍用with，不能只根据外层函数是否async决定。用户自行修订为async with lock，运行read A 0、write A 1、read B 1、write B 2、count 2，通过。见 [A07](notes/A07.md)。

## M33 · 仅在成功get之后完成记账（A07）

原队列消费者把await q.get放进try，finally无条件task_done；若get尚未成功即被中断，会为未取得的项误记完成。用户将get移到try之前，之后每次成功取得的数据与结束信号都经过finally记账，队列11项通过。最终用户正确预测empty True、joined False、joined True，并解释get腾容量不等于处理完成，完成记账复测通过。取消与失败生命周期留A08验证，未声称本课已执行取消实验。见 [A07](notes/A07.md)。
