# go-org 解析与写出链路分析

当前提交：`45e564da15ced4ad3d2c297c85a7dfeaea9113c8`。本文只核对当前源码与新增的最小测试；没有修改公开 AST、writer 结构或 writer 默认选项。

## 最小主线文档

```org
* Project
:PROPERTIES:
:CUSTOM_ID: project
:END:
See [[https://example.com][Example <tag> & more]] [fn:one].
** Details
#+ATTR_HTML: :class callout
#+BEGIN_FOO
=literal <markup>=
#+END_FOO
| Name | Count |
|------+-------|
| a    | 1     |
#+TBLFM: @2$2=@2$2+1
#+BEGIN_SRC go
if a < b && c != nil {
	fmt.Println("<raw>")
}
#+END_SRC

[fn:one] First note, including [fn:two].
* References
[fn:two] Second note.
```

## 扫描、建树和上下文

1. `Parse` 初始化 `Outline`、settings、named nodes、link/macro map 和 path，然后逐行 `tokenize`，最后递归 `parseMany` 建树；写回时先执行 writer 的 `Before`，再分派根节点并执行 `After`（`org/document.go:130`、`org/document.go:149`、`org/document.go:110`、`org/document.go:122`）。词法器按 headline、drawer、block、result、list、table、keyword、footnote 等顺序短路匹配（`org/document.go:66`）。
2. `* Project` 由 `lexHeadline` 捕获星号和标题文本，`parseHeadline` 用星号长度设置 `Lvl`，依次剥离 TODO、priority、COMMENT、tags，并把剩余标题交给 `parseInline`（`org/headline.go:37`、`org/headline.go:44`、`org/headline.go:71`）。
3. `parseHeadline` 递归 `parseMany` 时传入一个局部 stop：遇到同级或更高级 headline 即终止。第一个直接子节点若是 `PropertyDrawer`，会从普通 `Children` 中移出并挂到 `Headline.Properties`；这也意味着 property drawer 只在该精确位置被提升（`org/headline.go:74`、`org/headline.go:77`、`org/headline.go:79`）。
4. 属性 drawer 的 key 被统一转成大写，value 保留字符串；`PropertyDrawer.Get` 从结构化键值对中按顺序查找，`CUSTOM_ID` 因此不进入 HTML 正文，只参与 ID 计算（`org/drawer.go:62`、`org/drawer.go:73`、`org/drawer.go:84`）。
5. 段落聚合连续的 `text` token，按换行拼回后调用 `parseInline`；描述 link 在 `[[url][desc]]` 中拆出 URL，并把描述重新送进 inline parser，因此描述中的强调、实体字符和脚注都能形成嵌套节点（`org/paragraph.go:29`、`org/paragraph.go:39`、`org/inline.go:310`、`org/inline.go:321`）。
6. `#+ATTR_HTML` 与 `#+CAPTION` 不是普通裸 keyword：`parseAffiliated` 连续消费这些 keyword，把属性放入 `Metadata.HTMLAttributes`，再解析紧随其后的一个节点并包成 `NodeWithMeta`（`org/keyword.go:103`、`org/keyword.go:109`、`org/keyword.go:134`、`org/keyword.go:139`）。
7. `#+BEGIN_FOO` 形成 `Block{Name:"FOO"}`。只有 `SRC`、`EXAMPLE`、`EXPORT` 被当成 raw block；其他名称走 `parseMany`，所以主线 FOO 内的 `=literal <markup>=` 是 `Paragraph -> Emphasis(=) -> Text(IsRaw:true)`，不是整段原始字节（`org/block.go:69`、`org/block.go:79`、`org/block.go:87`、`org/block.go:89`）。
8. table 行先拆成字符串列，分隔行记为 nil row 并记录 `SeparatorIndices`；所有 cell 再调用 `parseInline`。列宽和对齐信息存为共享的 `ColumnInfo` 指针，数字列默认右对齐，特殊 `<l/c/rN>` 行才覆盖对齐（`org/table.go:46`、`org/table.go:50`、`org/table.go:55`、`org/table.go:68`、`org/table.go:100`、`org/table.go:120`）。
9. `#+TBLFM: ...` 在词法上是普通 keyword；它不是 `Table` 的字段，而是 table 后面的独立顶层 `Keyword{Key:"TBLFM"}`。源码没有公式求值入口，表格中的 `1` 保持字符串 cell（`org/keyword.go:56`、`org/keyword.go:81`、`org/table.go:10`、`org/table.go:70`）。
10. `#+BEGIN_SRC go` 的正文按原始行读取，只做缩进裁剪和 org/example 反引逗号处理，随后用 `parseRawInline` 切成 raw `Text` 与 `LineBreak`；结束块必须同名，`SRC` 还会尝试吸收后续 result（`org/block.go:73`、`org/block.go:81`、`org/block.go:84`、`org/block.go:93`、`org/block.go:96`）。
11. 脚注引用在 inline 阶段产生 `FootnoteLink`；具名 inline 定义会创建 `Inline:true` 的 `FootnoteDefinition`。独立 `[fn:name]` 行由 footnote parser 把首 token 重写成正文并递归解析，直到 headline、下一个脚注定义或两个空行（`org/inline.go:262`、`org/inline.go:268`、`org/inline.go:270`、`org/footnote.go:22`、`org/footnote.go:24`、`org/footnote.go:25`）。

主线 AST 形状可概括为：根节点是三个 `Headline`。`Project` 的 `Properties` 指向属性 drawer，正文段落包含 `RegularLink` 和 `FootnoteLink`；`Details` 嵌套在 `Project.Children` 中，其 children 依次是带 `NodeWithMeta` 的 `Block(FOO)`、`Table`、`Keyword(TBLFM)`、`Block(SRC)`、一个空段落和 `FootnoteDefinition(one)`；`References` 下是 `FootnoteDefinition(two)`。

上下文不是独立的 AST 字段：parser 通过函数参数和闭包向下传。`parseMany/parseOne` 接收父级 `stopFn`，headline 用自身 level 包住它，block 用同名 end 标记包住它；inline parser 始终带着同一个 `*Document`，因此能读取配置、宏、link map 和解析器回调。writer 侧则共享同一个 writer 实例，HTML writer 的 `document/htmlEscape/footnotes` 和 Org writer 的 `indent` 都在递归 `WriteNodes` 中持续生效。

## 两个 writer 在哪里分流

分流不发生在 parser：两种输出都遍历同一批节点，`WriteNodes` 根据动态类型调用 writer 接口方法（`org/writer.go:46`、`org/writer.go:60`、`org/writer.go:62`、`org/writer.go:82`、`org/writer.go:100`、`org/writer.go:106`）。

- HTML writer 在 `Before` 写 title/TOC，在 `After` 统一回填脚注；property drawer 被显式忽略，headline 写 outline container、heading 和 children 容器（`org/html_writer.go:114`、`org/html_writer.go:134`、`org/html_writer.go:140`、`org/html_writer.go:145`、`org/html_writer.go:294`、`org/html_writer.go:320`）。
- HTML 的 FOO block 走 default 分支，写成 `<div class="foo-block">`，并递归写出结构化 children；SRC block 先取回 raw content，再交给 `HighlightCodeBlock`，默认 highlighter 对 source 做 `html.EscapeString`（`org/html_writer.go:147`、`org/html_writer.go:163`、`org/html_writer.go:177`、`org/html_writer.go:627`、`org/html_writer.go:84`、`org/html_writer.go:88`）。
- HTML table 根据 separator 位置决定 `thead/tbody`，普通 cell 写 `td`，header cell 写 `th`，但不消费 `TBLFM` keyword（`org/html_writer.go:559`、`org/html_writer.go:561`、`org/html_writer.go:578`、`org/html_writer.go:590`）；普通 `Keyword` 只有 `HTML` 和 `TOC` 特殊处理，所以 `TBLFM` 在 HTML 中无输出（`org/html_writer.go:211`、`org/html_writer.go:212`、`org/html_writer.go:214`）。
- Org writer 的 headline 重建星号、状态、priority、title、tags、property drawer 和 children；FOO/SRC block 重建 `#+BEGIN_...`/`#+END_...`，table 重建等宽行，`TBLFM` 作为 keyword 原样写回（`org/org_writer.go:58`、`org/org_writer.go:81`、`org/org_writer.go:87`、`org/org_writer.go:96`、`org/org_writer.go:104`、`org/org_writer.go:265`、`org/org_writer.go:185`）。
- Org writer 不做 HTML 转义，`Text` 直接写出；它也不收集脚注，footnote definition 就在 AST 所在位置写出，footnote link 写成 `[fn:name]`（`org/org_writer.go:311`、`org/org_writer.go:158`、`org/org_writer.go:354`）。

## 不变量

1. **Source position 只在逐行扫描期短暂存在。** token 只保存 `kind/lvl/content/matches`，没有行号字段；公开 AST 类型也没有 position 字段（`org/document.go:59`、`org/document.go:36`、`org/headline.go:22`、`org/paragraph.go:9`）。`block.Children` 里的 raw `Text` 是去缩进后的内容，不能用它反推出原始字节偏移；这一结论由字段定义静态核对，而不是运行时猜测。
2. **锚点优先用未清洗的 `CUSTOM_ID`，否则用未排除 headline 的序号。** `Headline.ID` 直接返回属性值，没有 `CUSTOM_ID` 才返回 `headline-Index`（`org/headline.go:103`、`org/headline.go:104`、`org/headline.go:107`）；`addHeadline` 只对非 excluded headline 增加序号（`org/document.go:264`、`org/document.go:267`）。HTML 中 TOC、section、heading 都调用同一个 `ID()`（`org/html_writer.go:279`、`org/html_writer.go:301`、`org/html_writer.go:302`）。
3. **HTML 普通文本默认按实体规则转义，raw text 仍经 HTML 字符串转义。** `WriteText` 在普通上下文调用 entity replacer 后 `html.EscapeString`，在 `Text.IsRaw` 或关闭 entity 选项时仍调用 `html.EscapeString`（`org/html_writer.go:326`、`org/html_writer.go:329`、`org/html_writer.go:332`）；raw block 先关闭普通 text writer 的 entity/直接写出路径，再由 highlighter 或 EXAMPLE 分支显式 escape（`org/html_writer.go:627`、`org/html_writer.go:630`、`org/html_writer.go:631`、`org/html_writer.go:166`）。
4. **脚注按引用顺序编号并延迟到文档末尾回填。** 具名重复引用返回同一索引，未命名脚注每次追加；先出现定义时先存 `unused`，之后第一个引用会把它接上（`org/html_writer.go:665`、`org/html_writer.go:666`、`org/html_writer.go:670`、`org/html_writer.go:677`、`org/html_writer.go:679`）。定义节点在正文遍历时只更新状态不输出，最终 `WriteFootnotes` 再遍历 list；定义中新增脚注会扩展 list（`org/html_writer.go:226`、`org/html_writer.go:230`、`org/html_writer.go:238`、`org/html_writer.go:255`）。
5. **未知 block 保留块包装和大写名称，但不承诺逐字透明。** parser 用同一 `Block` 节点承载已知和未知名称，结束标记要求同名；raw 名称集合是固定的 `SRC/EXAMPLE/EXPORT`（`org/block.go:37`、`org/block.go:71`、`org/block.go:78`、`org/block.go:93`、`org/block.go:69`）。Org writer 总能重建 begin/end wrapper（`org/org_writer.go:87`、`org/org_writer.go:104`）；HTML writer default 分支也保留 `name-block` 包装（`org/html_writer.go:176`、`org/html_writer.go:178`）。

## 五个潜在风险

### 1. 未知 block 的“透明性”在 Org 往返后仍无法恢复

- 最小输入：

  ```org
  #+BEGIN_FOO
  * Headline inside block
  #+END_FOO
  ```

- 兼容性预期：未知 block 被当作不透明扩展块保留，内部 headline 不进入文档 outline；或者至少在写出 Org 后，消费者能从 AST 明确判断这段内容未按 Org 正文解释。
- 实际代码路径：`isRawTextBlock("FOO")` 为 false，parser 走 `parseMany`，内部 headline 被递归解析并调用 `addHeadline` 进入全局 outline（`org/block.go:69`、`org/block.go:89`、`org/headline.go:71`、`org/document.go:264`）。Org writer 会原样重建 wrapper，重新解析仍得到 `Block -> Headline`，所以文本往返稳定，但“未知内容未被解释”的结构不变量已经不存在；若第一遍解析后程序依赖 `Outline`，影响已经发生。
- 回归覆盖：新增测试只保证良性单行内容的 wrapper 和块名在 parse -> Org -> reparse 后仍存在，不把整段输出当 golden，也不断言未知 block 是 raw（`org/understanding_test.go:8`、`org/understanding_test.go:22`、`org/understanding_test.go:30`、`org/understanding_test.go:38`）。

### 2. `TBLFM` 不是 table 的子结构

- 最小输入：

  ```org
  | a |
  |---|
  | 1 |
  #+TBLFM: @2$1=@2$1+1
  ```

- 预期：公式在语义上属于紧邻 table；复制、重排或局部写出一个 table 时，公式仍随 table 保留；需要导出计算结果时有明确入口。
- 实际代码路径：table parser 在遇到非 table token 时结束，只返回 `Table`；`#+TBLFM` 随后由 keyword parser 存到 `BufferSettings` 并作为独立 `Keyword` 节点返回（`org/table.go:48`、`org/table.go:58`、`org/table.go:77`、`org/keyword.go:75`、`org/keyword.go:81`、`org/keyword.go:87`）。当前 writer 只重写 keyword 文本，HTML writer 默认不输出普通 `TBLFM`；源码中没有公式求值调用。这能规范地保留文本，但不能提供 table-formula 的结构关系。

### 3. `ATTR_HTML` 只能可靠地挂到单个 HTML 根节点

- 最小输入：

  ```org
  #+ATTR_HTML: :class callout
  #+BEGIN_EXPORT html
  <p>a</p><p>b</p>
  #+END_EXPORT
  ```

- 预期：class 被应用到导出的两个段落（例如分别添加 class，或包裹一个共同容器），至少报告无法表示。
- 实际代码路径：`NodeWithMeta` 先把目标节点渲染成字符串，再对每组属性调用 `withHTMLAttributes`（`org/html_writer.go:532`、`org/html_writer.go:539`）。后者用 `<body>` 作为 context 调 `html.ParseFragment`，当结果不是恰好一个节点时记录日志并返回未修改 input（`org/html_writer.go:604`、`org/html_writer.go:609`、`org/html_writer.go:610`、`org/html_writer.go:611`、`org/html_writer.go:612`）。因此两个 `<p>` 会原样输出，`:class callout` 被静默丢弃。

### 4. 跨 headline 的脚注定义会受 exclusion 影响，引用可能悬空

- 最小输入：

  ```org
  * Visible
  See [fn:x].
  * Hidden :noexport:
  [fn:x] body
  ```

- 预期：跨段命名脚注仍能回填；或者当定义所在 headline 被排除时，连引用也一起不可见，避免产生没有定义的编号。
- 实际代码路径：脚注定义在正文遍历到该节点时通过 `WriteFootnoteDefinition -> updateDefinition` 注册（`org/html_writer.go:226`、`org/html_writer.go:685`）。但 excluded headline 在 `WriteHeadline` 入口直接返回，整棵子树不遍历（`org/html_writer.go:294`、`org/html_writer.go:295`）；可见正文中的 `WriteFootnoteLink` 已经创建编号和占位（`org/html_writer.go:366`、`org/html_writer.go:370`、`org/html_writer.go:677`）。最终该槽位为 nil，只记录 “Missing footnote definition” 日志并跳过定义体（`org/html_writer.go:239`、`org/html_writer.go:242`、`org/html_writer.go:249`）。主线文档的跨普通 headline 回填能工作，因为 `fn:two` 的定义节点会被正常遍历到；风险只在定义被排除或未写出时出现。

### 5. `CUSTOM_ID` 未转义即进入 HTML 属性上下文

- 最小输入：

  ```org
  * X
  :PROPERTIES:
  :CUSTOM_ID: a" onclick="bad
  :END:
  ```

- 预期：ID 应被规范化或 HTML attribute-escape，重复/非法 ID 也应有稳定策略。
- 实际代码路径：`PropertyDrawer.Get` 原样返回属性值，`Headline.ID` 直接把它作为 ID（`org/drawer.go:88`、`org/drawer.go:89`、`org/headline.go:103`、`org/headline.go:104`）。HTML 用 `fmt.Sprintf` 将该值插入 TOC 的 `href`、container `id` 和 heading `id`，没有经 `html.EscapeString` 或 URL escaping（`org/html_writer.go:279`、`org/html_writer.go:301`、`org/html_writer.go:302`）。普通文本和 URL 有转义路径，但 `CUSTOM_ID` 不走这些路径（`org/html_writer.go:326`、`org/html_writer.go:391`、`org/html_writer.go:392`）。

## 推测与边界

- “Org mode 上游通常如何处理不透明未知 block、table formula 求值或非法 CUSTOM_ID”没有在本次任务中核对上游文档；上文把这些标为兼容性预期，而不是源码既定事实。
- 源码能证明 `html.ParseFragment` 会处理标准 HTML 属性转义，但本文没有枚举全部浏览器解析差异；多根节点丢属性的结论直接依赖 `len(nodes) != 1` 分支。
- 本文没有声称所有 Org -> Org 输入都是幂等文本；新增回归只证明未知 block 的包装器在一个最小良性输入上保留。列表缩进、原始空白和特殊对齐行等其它格式化差异未纳入本次结论。

## 复现命令

以下命令均从仓库根目录运行，不需要外部服务、公网或手工环境变量。

```sh
go build ./...
go test ./...
go test ./org -run 'Understanding' -count=1 -v
```

主线文档可写入 `/tmp/go-org-main.org` 后用 CLI 检查两个 writer：

```sh
go run . render /tmp/go-org-main.org html
go run . render /tmp/go-org-main.org org
```

风险 3 的最小复现：

```sh
cat >/tmp/go-org-attr.org <<'EOF'
#+ATTR_HTML: :class callout
#+BEGIN_EXPORT html
<p>a</p><p>b</p>
#+END_EXPORT
EOF
go run . render /tmp/go-org-attr.org html
```

当前路径输出两个未带 `callout` 的 `<p>`，同时日志记录 `withHTMLAttributes: Could not extend attributes`。

风险 4 的最小复现：

```sh
cat >/tmp/go-org-footnote.org <<'EOF'
* Visible
See [fn:x].
* Hidden :noexport:
[fn:x] body
EOF
go run . render /tmp/go-org-footnote.org html
```

当前路径输出编号 1 的引用，但脚注定义列表为空；日志记录 `Missing footnote definition for [fn:x] (#1)`。

## 本次验证输出摘要

- 准备阶段：`go build ./...` 退出码 0。
- 全量测试：`go test ./...` 退出码 0；`blorg` 与 `org` 包均 `ok`，根包无测试文件。
- 指定验收：`go test ./org -run 'Understanding' -count=1 -v` 退出码 0，并显示 `TestUnderstandingUnknownBlockPreservesWrapperAcrossOrgRoundTrip` 与 `--- PASS`。
- 新增测试位置：`org/understanding_test.go:8`。它只检查 `Block.Name`、首个 child 的节点类型，以及 Org 写回再解析后仍是同名 `Block`，没有使用整份主线文档 golden。
