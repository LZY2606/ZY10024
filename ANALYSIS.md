# go-org 解析与写出分析

基准提交：`45e564d Initial environment snapshot (ZY10024)`。本次只新增分析文档和一个包内回归测试，未修改公开 AST，也未修改 `NewHTMLWriter()` 或 `NewOrgWriter()` 的默认选项。

## 主线文档

下面的最小输入同时覆盖嵌套 headline、属性 drawer、表格与公式、代码块、未知 block、带描述 link 和跨段脚注。后文以它逐段追踪：

```org
* Overview :tag:
:PROPERTIES:
:CUSTOM_ID: overview
:END:
See [[https://example.com][Example]][fn:1].
#+ATTR_HTML: :class compact :id totals
| a | b |
|---+---|
| 1 | 2 |
#+TBLFM: @2$2=@2$1+1
** Detail
#+BEGIN_SRC go
fmt.Println("<b>raw</b>")
#+END_SRC
#+BEGIN_WIDGET
body
#+END_WIDGET
[fn:1] Cross-section note.
```

## 扫描与上下文

- 每行先经过 `bufio.Scanner`；扫描结果只追加到 `Document.tokens`，token 结构只有 `kind/lvl/content/matches`，没有行号、列号或字节偏移。见 `org/document.go:39`、`org/document.go:59`、`org/document.go:161`、`org/document.go:165`。
- 词法函数按 `lexFns` 顺序尝试；本主线依次命中 headline、drawer、text、keyword、table、table separator、block、footnote definition。顺序定义在 `org/document.go:66`，分发在 `org/document.go:215`。
- `Parse` 从 token 0 调 `parseMany`；每个 parser 返回“消耗 token 数 + 节点”。这个计数是解析期间的相对位置，不是暴露到 AST 的 source position。见 `org/document.go:149`、`org/document.go:254`、`org/document.go:257`。
- headline 的停止条件是后续同级或更高层级 headline；它递归调用 `parseMany`，因此属性 drawer、段落、表格、二级标题和脚注定义都作为树节点归属到对应 section。见 `org/headline.go:74`、`org/headline.go:77`、`org/headline.go:84`。
- 属性 drawer 只有作为 headline 的第一个 child 时才提升为 `Headline.Properties`，随后从普通 `Children` 移除。见 `org/headline.go:78`、`org/headline.go:80`、`org/drawer.go:62`、`org/drawer.go:73`。
- paragraph 把连续 text token 用换行连接，缩进量由当前 `baseLvl` 还原，然后整段交给 inline parser。见 `org/paragraph.go:30`、`org/paragraph.go:35`、`org/paragraph.go:39`。
- inline parser 逐字节扫描普通文本，遇到起始字符时分发给 link、脚注、emphasis、macro 等；未识别字符成为相邻 `Text`。见 `org/inline.go:90`、`org/inline.go:94`、`org/inline.go:121`、`org/inline.go:135`。
- `[[URL][description]]` 以第一个 `]][` 分拆，描述部分递归解析 inline；再按第一个冒号拆 protocol，默认 resolver 返回 `RegularLink`。见 `org/inline.go:310`、`org/inline.go:319`、`org/inline.go:322`、`org/inline.go:328`、`org/document.go:97`。
- `[fn:1]` 在 inline parser 中成为 `FootnoteLink{Name:"1", Definition:nil}`；定义标记本身则在块级 parser 中成为 `FootnoteDefinition`。见 `org/inline.go:244`、`org/inline.go:262`、`org/inline.go:268`、`org/footnote.go:22`。
- 表格 parser 连续消费 row 和 separator；每个 cell trim 后递归 `parseInline`。列宽、默认右对齐和特殊对齐行从 raw row 推导。见 `org/table.go:46`、`org/table.go:50`、`org/table.go:56`、`org/table.go:70`、`org/table.go:80`、`org/table.go:120`。
- `#+ATTR_HTML` 与其后一个节点由 `parseAffiliated` 包成 `NodeWithMeta`；`#+TBLFM` 不在表格 parser 内，解析为同级 `Keyword`，同时写入 `BufferSettings`。见 `org/keyword.go:75`、`org/keyword.go:103`、`org/keyword.go:126`、`org/keyword.go:139`、`org/keyword.go:81`。
- `BEGIN_SRC` 和未知 `BEGIN_WIDGET` 都产生 `Block`，但只有 `SRC`、`EXAMPLE`、`EXPORT` 走 raw inline；其他 block 的内部继续递归解析。见 `org/block.go:69`、`org/block.go:71`、`org/block.go:79`、`org/block.go:87`、`org/block.go:88`。

主线解析后的关键 AST 形状如下：

- 根节点是一个一级 `Headline`；`Properties.Get("CUSTOM_ID")` 为 `overview`，其 `Children` 包含段落、带 meta 的表格、`TBLFM` keyword、二级 headline。
- 段落 children 顺序为 `Text("See ")`、`RegularLink`、`FootnoteLink`、`Text(".")`。
- 二级 headline 的 children 包含 `Block{Name:"SRC"}`、`Block{Name:"WIDGET"}` 和 `FootnoteDefinition{Name:"1"}`；脚注定义在树中位于二级 section，但 HTML writer 会在文档末尾统一回填。

## Writer 分流

`Document.Write` 只负责生命周期：检查 parse 错误、调用 writer 的 `Before`、经 `WriteNodes` 做动态类型分发、调用 `After`。具体格式完全由各自 writer 决定。见 `org/document.go:110`、`org/document.go:122`、`org/document.go:124`、`org/writer.go:46`。

### HTML

- `Before` 写 title 和 TOC；默认 `toc:t` 会基于 `Outline` 输出导航。默认一级 Org headline 对应 `<h2>`，因为 `TopLevelHLevel` 默认为 2。见 `org/html_writer.go:78`、`org/html_writer.go:90`、`org/html_writer.go:114`、`org/html_writer.go:134`、`org/html_writer.go:261`。
- headline 写出外层 outline div、heading、title/tags、children div；属性 drawer 不直接输出。见 `org/html_writer.go:144`、`org/html_writer.go:294`、`org/html_writer.go:301`、`org/html_writer.go:320`。
- `NodeWithMeta` 先把被修饰节点渲染成字符串，再用 HTML parser 解析这一个根元素并追加属性，caption 再包成 `figure`。见 `org/html_writer.go:532`、`org/html_writer.go:539`、`org/html_writer.go:550`、`org/html_writer.go:604`。
- `SRC` block 临时关闭逐文本转义以取得代码内容，再由默认 highlighter 对 source 调 `html.EscapeString`；未知 block 的普通 children 仍按常规 HTML 节点渲染。见 `org/html_writer.go:147`、`org/html_writer.go:163`、`org/html_writer.go:176`、`org/html_writer.go:627`。
- 普通文本按 `html.EscapeString` 输出；启用实体选项时先替换实体再转义。raw `Text.IsRaw` 只跳过实体替换，不跳过 HTML 转义。见 `org/html_writer.go:326`、`org/html_writer.go:329`、`org/html_writer.go:332`。
- link URL 在 `WriteRegularLink` 起始即转义；带描述 link 输出普通 `<a href="...">description</a>`。见 `org/html_writer.go:391`、`org/html_writer.go:392`、`org/html_writer.go:415`。
- footnote link 在正文写引用编号；footnote definition 在遍历时只更新 writer 状态，真正列表在 `After` 的 `WriteFootnotes` 输出。见 `org/html_writer.go:140`、`org/html_writer.go:226`、`org/html_writer.go:230`、`org/html_writer.go:366`。

### Org

- Org writer 的 `Before/After` 为空，按 AST 顺序重写 headline、drawer、block、keyword 和脚注，不做 HTML 的 TOC 预输出或脚注后处理。见 `org/org_writer.go:46`、`org/org_writer.go:58`、`org/org_writer.go:87`、`org/org_writer.go:185`。
- headline 重建星号、TODO、优先级、title、tag column，并单独写 property drawer。见 `org/org_writer.go:58`、`org/org_writer.go:81`、`org/org_writer.go:146`。
- block 保留大写名称和参数；raw block 直接写 raw children，普通 block 递归写 children 后补结束标记。见 `org/org_writer.go:87`、`org/org_writer.go:93`、`org/org_writer.go:96`、`org/org_writer.go:104`。
- `NodeWithMeta` 会把 caption 和 `ATTR_HTML` 原样重写成 affiliated keywords，再写被修饰节点；因此本主线的表格与 `TBLFM` 的相对顺序可由 AST 顺序保留。见 `org/org_writer.go:197`、`org/org_writer.go:203`、`org/org_writer.go:207`。
- 表格按 `ColumnInfos` 重新排版；separator 被重建成由 `-` 和 `+` 组成的规则线。见 `org/org_writer.go:265`、`org/org_writer.go:268`、`org/org_writer.go:286`、`org/org_writer.go:303`。
- 普通文本不做 HTML 转义；link 用双方括号重建，脚注引用和脚注定义分别按 Org 语法写出。见 `org/org_writer.go:311`、`org/org_writer.go:354`、`org/org_writer.go:363`、`org/org_writer.go:158`。

## 不变量

### Source position

1. 词法阶段保留的是“当前行”的正则捕获和缩进，不是持久位置；`tokenize` 每次收到一行并立即生成 token，scanner 的行号没有存入 token。见 `org/document.go:161`、`org/document.go:165`。
2. 解析阶段只维护 token slice 下标：`parseMany` 从 `i` 前进，子 parser 返回相对 `start` 的消耗数。下标不进入任何公开节点。见 `org/document.go:254`、`org/document.go:258`。
3. 对下游可依赖的位置只有树位置（父子、children 顺序）和逻辑层级（`Headline.Lvl`、`token.lvl`），不能宣称 AST 可回到输入行列。`Headline` 字段定义见 `org/headline.go:22`，缩进消费见 `org/block.go:147`。

### 锚点生成

1. 有 `CUSTOM_ID` 属性时，`ID()` 原样返回属性值；没有时返回 `headline-<Index>`。见 `org/headline.go:103`、`org/headline.go:104`、`org/headline.go:107`。
2. `Index` 是 outline 计数，但排除的 headline 不增加计数；outline 树仍添加该 section。见 `org/document.go:264`、`org/document.go:267`、`org/headline.go:124`。
3. TOC 和正文都调用同一个 `ID()`，所以默认情况下 TOC 链接目标与正文 heading `id` 一致。见 `org/html_writer.go:278`、`org/html_writer.go:301`、`org/html_writer.go:302`。

### 转义

1. HTML 普通文本必须经 `html.EscapeString`；raw code text 的特殊点是仍经 highlighter 转义，只是不做 Org entity 替换。见 `org/html_writer.go:326`、`org/html_writer.go:329`、`org/html_writer.go:84`、`org/html_writer.go:88`。
2. `EXPORT html` 和 `HTML` keyword 是显式 HTML 通道，写出时不转义；这是受输入信任的原始 HTML，不应与普通文本混淆。见 `org/html_writer.go:167`、`org/html_writer.go:169`、`org/html_writer.go:211`、`org/html_writer.go:213`。
3. Org writer 只负责重建 Org 语法，不把 `<`、`>`、`&` 转成 HTML entities；同一 raw code 片段写回 Org 后仍是源码文本。见 `org/org_writer.go:311` 和 `org/org_writer.go:96`。

### 脚注回填

1. 引用在 inline 阶段成为 `FootnoteLink`；具名 inline definition 会直接挂到 link，匿名 inline definition 的 `Name` 为空。见 `org/inline.go:262`、`org/inline.go:268`、`org/inline.go:270`。
2. 独立定义的第一行被替换成其内容 token 后递归解析；定义在遇到 headline、另一个定义或两个空行时结束。见 `org/footnote.go:22`、`org/footnote.go:24`、`org/footnote.go:25`、`org/footnote.go:30`。
3. HTML 输出期间，具名引用第一次出现时占用编号；重复具名引用复用编号，inline definition 直接随 link 入列。见 `org/html_writer.go:665`、`org/html_writer.go:666`、`org/html_writer.go:677`、`org/html_writer.go:679`。
4. 先引用、后定义时，`updateDefinition` 用同名槽位回填；先定义、后引用时放入 `unused`，引用出现后再挂上。见 `org/html_writer.go:670`、`org/html_writer.go:685`、`org/html_writer.go:687`、`org/html_writer.go:691`。
5. 最终脚注列表只在 HTML `After` 输出；缺失定义会记录日志并跳过该项，不把定义按其 AST 所在位置就地输出。见 `org/html_writer.go:140`、`org/html_writer.go:238`、`org/html_writer.go:242`、`org/html_writer.go:249`。

### 未知 block 保留

1. 任意匹配 `#+BEGIN_<word>` 的名称都会成为 `Block`，名称在词法阶段大写；未知名称不是错误。见 `org/block.go:30`、`org/block.go:37`、`org/block.go:39`、`org/block.go:71`。
2. 只有 `SRC`、`EXAMPLE`、`EXPORT` 被视为 raw text；未知 block 的 children 按普通 Org AST 递归构造。见 `org/block.go:69`、`org/block.go:79`、`org/block.go:88`。
3. HTML 默认给未知 block 输出 `<div class="<lower-name>-block">`；Org writer 用同名 `BEGIN/END` 包回 children。见 `org/html_writer.go:176`、`org/html_writer.go:177`、`org/org_writer.go:87`、`org/org_writer.go:104`。
4. 因而当前可回归保证的是“wrapper 和解析后 child 结构”保留；不能把它表述为未知 block 内部源码字节逐字保留。

## 潜在风险

### 1. `TBLFM` 不是表格字段

- 最小输入：

```org
| 1 |
#+TBLFM: @1$1=@1$1+1
```

- 预期（按“公式属于前一张表”的使用直觉）：AST 中表格节点携带公式，或 writer 明确维护表格与公式的关联。
- 实际代码路径：表格 parser 遇到非 row/separator token 即停止，见 `org/table.go:48`、`org/table.go:58`；keyword parser 把 `TBLFM` 变成独立 `Keyword` 并写入 `BufferSettings`，见 `org/keyword.go:81`。HTML writer 对 `TBLFM` 没有分支，见 `org/html_writer.go:211`；Org writer 只按同级顺序重写 keyword，见 `org/org_writer.go:185`。
- 可核对结果：主线 AST 中 `Table` 与 `Keyword{Key:"TBLFM"}` 是兄弟节点，公式不在 `Table` 结构内。结构写回 Org 后目前仍按相邻顺序保留；这是顺序依赖，而不是强引用。
- 推测：未来若有节点重排、过滤导出或在表格与公式之间插入内容，公式可能与目标表脱离。此后果未在当前测试中证明。

### 2. 未知 block 的结构往返只覆盖 wrapper

- 最小输入：

```org
#+BEGIN_WIDGET
* Not a heading
body
#+END_WIDGET
```

- 预期（按“未知 block 应保留内部原文”的直觉）：block children 为 raw text，写回后内部仍是 block 内的字面文本。
- 实际代码路径：`WIDGET` 不满足 raw block 判定，进入 `parseMany`，见 `org/block.go:69`、`org/block.go:88`；内部 `* Not a heading` 被构造成 `Headline`。Org writer 递归写 headline，见 `org/org_writer.go:84`、`org/org_writer.go:87`。
- 可核对结果：一次解析的 `Block.Name` 为 `WIDGET`，但第一个 child 是 `Headline`；写回 Org 再解析后 wrapper 仍是 `WIDGET`，child 仍是 headline。
- 新增回归测试锁定的是当前更窄的不变量：普通 body（`Body with /emphasis/.`）解析、写回、再解析后仍是一个顶层 `WIDGET` block 且有一个 child。它不是整份样例 golden，也不断言 raw 字节。

### 3. `ATTR_HTML` 只修饰一个根节点，result 是兄弟输出

- 最小输入：

```org
#+ATTR_HTML: :id code-result
#+BEGIN_SRC go :exports both
fmt.Println(1)
#+END_SRC

#+RESULTS:
| 1 |
```

- 预期（一种合理但需确认的 HTML 上下文预期）：属性同时作用于源码块和结果，或至少结果表格带同一个容器/id。
- 实际代码路径：affiliated parser 只把紧随其后的一个 `Block` 包入 `NodeWithMeta`，见 `org/keyword.go:134`、`org/keyword.go:139`；SRC block 在解析后把 `#\+RESULTS` 放进 `Block.Result`，见 `org/block.go:96`、`org/block.go:97`；HTML `WriteNodeWithMeta` 只给渲染 block 得到的一个根元素加属性，见 `org/html_writer.go:532`、`org/html_writer.go:539`、`org/html_writer.go:604`；之后 `WriteBlock` 另行输出 result，见 `org/html_writer.go:181`。
- 可核对结果：源码 `<div class="src src-go">` 是被修饰字符串的根节点；随后的 `<table>` 没有 `id="code-result"`。
- 推测：Org 官方语法可能把 affiliated attribute 定义为只属于下一个元素；若是这样，当前行为符合语法而不满足上述“包裹代码+结果”的直觉。本文不把该规范判断写成事实。

### 4. 排除 section 中的脚注定义会被整棵剪枝

- 最小输入：

```org
* Visible [fn:x]
* Hidden :noexport:
[fn:x] definition inside hidden section
* Also visible
```

- 预期（跨段引用角度）：正文引用仍能在文末回填到同名定义，或给出明确的未定义引用。
- 实际代码路径：headline parser 先递归把脚注定义放入 hidden headline 的 children，见 `org/headline.go:77`、`org/headline.go:84`；HTML writer 遇到 excluded headline 直接返回，不遍历其 children，见 `org/html_writer.go:294`、`org/html_writer.go:295`；因此 `WriteFootnoteDefinition` 不会执行，回填映射没有定义。最终缺失定义只在写脚注列表时记录日志，见 `org/html_writer.go:242`、`org/html_writer.go:249`。
- 可核对结果：正文有 `href="#footnote-1"` 的引用，但文末没有 `id="footnote-1"` 的定义；这是跨段/跨 section 引用与 headline 导出过滤的交互风险。

### 5. `CUSTOM_ID` 原样使用且不检查重复

- 最小输入：

```org
* First
:PROPERTIES:
:CUSTOM_ID: same
:END:
* Second
:PROPERTIES:
:CUSTOM_ID: same
:END:
```

属性保留字符的最小变体：

```org
* X
:PROPERTIES:
:CUSTOM_ID: a"onclick="alert(1)
:END:
```

- 预期：两个 heading 的锚点可区分，或解析/导出阶段报告重复 id；属性值中的保留字符也应按 HTML 属性上下文处理。
- 实际代码路径：property drawer 保存原值，见 `org/drawer.go:73`、`org/drawer.go:74`；`ID()` 直接返回 `CUSTOM_ID`，见 `org/headline.go:103`、`org/headline.go:104`；HTML 用 `fmt.Sprintf` 放入容器和 heading id，见 `org/html_writer.go:301`、`org/html_writer.go:302`，TOC 也引用同一字符串，见 `org/html_writer.go:278`。
- 可核对结果：重复 custom id 会生成重复 HTML id；带引号的变体会生成 `<h2 id="a"onclick="alert(1)">` 和同样形态的 TOC `href`。本文不把它进一步定性为特定安全模型下的 XSS，只把它列为属性上下文未转义的风险。

## 回归测试

新增测试：`TestUnderstandingUnknownBlockRoundTrip`，位置 `org/understanding_test.go:8`。

测试只做结构断言：

- 第一次 parse 后断言根节点是 `Block`、名称是 `WIDGET`、children 数为 1，见 `org/understanding_test.go:15`。
- 进一步断言唯一 child 是 `Paragraph`，且中间 inline 节点是 `/emphasis/` 对应的 `Emphasis`，见 `org/understanding_test.go:22`、`org/understanding_test.go:26`。
- 用 `NewOrgWriter()` 写出，再第二次 parse，见 `org/understanding_test.go:30`、`org/understanding_test.go:34`。
- 第二次 parse 后仍只有一个顶层节点，且仍是有一个 child 的 `WIDGET` block，见 `org/understanding_test.go:39`、`org/understanding_test.go:43`。

这个测试证明的是未知 block wrapper 与 child 数量的结构稳定性；它没有把整份主线文档做成大段 golden 字符串，也没有声称未知 block 内部是 raw byte preservation。

## 复现

以下命令均从仓库根目录直接运行，不需要外部服务、公网或手工环境变量。

准备阶段（不计入演示）：

```sh
go build ./...
```

验收命令：

```sh
go test ./org -run 'Understanding' -count=1 -v
```

本次测试输出摘要：

```text
=== RUN   TestUnderstandingUnknownBlockRoundTrip
--- PASS: TestUnderstandingUnknownBlockRoundTrip (0.00s)
PASS
ok  	github.com/niklasfasching/go-org/org	0.005s
```

## 事实边界

- AST 形状、写出片段、行号引用和新增测试结果均由当前工作树源码或上述测试核对。
- 标注为“推测”的两项分别是：未来重排可能使 `TBLFM` 与表格脱离；Org 官方 affiliated attribute 语义可能本来就只要求修饰下一个元素。它们不是当前代码已证明的故障。
- 本文没有建议修改公开 AST 或 writer 默认值；若未来要补 source position、公式字段或 raw unknown block，需要单独设计兼容性。
