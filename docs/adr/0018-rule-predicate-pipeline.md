# ADR-0018: 规则条件解耦为独立算子与流水线求值 (Decoupled Predicates and Pipeline Evaluation)

## 状态

已通过 (Accepted)

## 背景

原有规则条件求值（`internal/rules/eval.go`）采用面向字段的集中式过程实现：
- 结构体 `compiledCond` 将 IP 前缀树、字符串匹配集合、正则表达式、数值阈值及出现频次等异构状态塞在同一个大结构体中；
- 编译与求值高度依赖针对 `field` 和 `operator` 的多层 `switch-case`，新增字段或运算符必须侵入核心引擎代码；
- 规则模拟器在 `handleSimulateRules` 中遗漏了画像富化器（`FactEnricher`）与前置拦截器（`RuleInterceptor`）的执行，导致外部插件扩展生效时模拟器推演与线上执行产生行为漂移。

## 决策

1. **规约与原子谓词抽象（Specification / Predicate Pattern）**：
   - 提取 `ConditionPredicate` 接口，定义 `Type()`、`Cost()`、`Match(ctx)` 与 `Trace(ctx)` 方法；
   - 将具体条件拆解为单一职责的原子算子：`IPRadixPredicate`（IP/CIDR 基数树）、`StringPredicate`（字符串常量与枚举集合）、`RegexPredicate`（预编译正则）、`NumericPredicate`（数值大小比较）、`DuplicatedPredicate`（重复频次判定）；
   - 每个算子自包含匹配与诊断留痕，彻底消除上帝结构体。

2. **条件编译开放注册表（PredicateRegistry）**：
   - 提供 `RegisterPredicateCompiler`、`RegisterPredicateCompilerPrefix` 与 `RegisterFallbackCompiler`；
   - 外部衍生工程或插件可动态注册新条件类型（如自定义 HTTP 标头 `header:X-Token` 或机房反爬探针），编译时自动分发至对应算子工厂，核心引擎对扩展开放、对修改关闭。

3. **条件流水线执行器（ConditionPipeline）与成本感知短路（Cost-Aware Short-Circuit）**：
   - 单条规则的多条件编译为 `ConditionPipeline`；
   - **线上热路径有两条路径，语义必须等价**：**扁平形态**（`rules.logic` + 一层 `conditions`）走 `ConditionPipeline`——根据 `pred.Cost()` 自动排序（便宜的常量/IP 匹配先执行，昂贵的正则后执行），AND 遇到首个 `false`、OR 遇到首个 `true` 时立即短路中断；**树形态**（`group` / `children`）走 `compiledNode` 递归求值，不做流水线排序。
   - **模拟器留痕不走 `pipeline.Trace`**：`POST /api/rules/simulate` 的逐条留痕走统一的 `compiledCond` 路径（`traceRule` → `collectTraces` 遍历 `compiledNode` 全量叶子，不按短路停下），保证"诊断说的"与"线上判的"同源；`ConditionPredicate.Trace` 目前只是单元测试用的诊断辅助，不在模拟器链路上。

4. **模拟器同源链路对齐**：
   - 在 `internal/httpapi/rules.go` 的 `handleSimulateRules` 中完整补齐 `vctx.ApplyEnrichers` 与 `rules.CheckInterceptors`；
   - 当触发前置拦截时，模拟器如实呈现熔断状态与留痕，杜绝线上与诊断分叉。

### 扁平与树语义等价的验收点

两条形态编译进同一个 `Compiled` 后必须对同一份画像给出完全一致的命中结论，本次统一了四处历史分叉：

1. **数值空白**：数值运算符（`gt` / `lt`）的字段值与阈值两侧都按 `TrimSpace` 后的形态解析，非数值恒不命中——流水线谓词与 `compiledCond` 同一口径。
2. **大小写折叠**：字符串运算符（`in` / `eq` / `contains` / `starts_with` / `ends_with` 等）两侧都做大小写折叠匹配，`compiledCond` 与 `StringPredicate` 不得一个折叠、一个不折叠。
3. **`duplicated` 多值**：`duplicated` 在动态字段（同名请求头 / 重复查询参数）上的多值语义与其余运算符一致——流水线谓词与 `compiledCond` 都遍历全部取值；计数来源 `Fact.Seen` 恒为 nil 时两形态都恒不命中。
4. **ip 谓词回落**：`ip` 字段上编译不出谓词的条件（如 `ip gt/lt/duplicated`）不得让流水线静默丢掉该叶子——只要有一条被 `compileCond` 接受、却编译不出谓词的叶子，整条规则**回落到树求值**，宁可放弃流水线也不留两套语义。

契约由 `internal/rules/flat_tree_equivalence_test.go`（逐字段逐运算符比对两形态命中结果）与 `internal/rules/pipeline_contract_test.go` 固化。

## 代价与后果

- 新增接口间接调用（`interface dispatch`），但经基准测试实测，单条规则纳秒级内存求值延迟基本无感（`85ns ~ 340ns` 量级），换取了完全解耦与可插拔性；
- 树状复杂嵌套组与扁平条件模型保持 100% 向后兼容，无需改动现有数据库与 API 协议；
- 契约由 `internal/rules/pipeline_contract_test.go` 固化保障。
