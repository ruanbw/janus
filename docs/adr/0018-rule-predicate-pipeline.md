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
   - **线上热路径**：根据 `pred.Cost()` 自动排序（便宜的常量/IP匹配先执行，昂贵的正则后执行），在 AND 遇到首个 `false`、OR 遇到首个 `true` 时立即短路中断，保证纳秒级求值与零无谓计算；
   - **模拟器诊断**：调用 `pipeline.Trace(ctx)` 走完全量流水线（不短路），精准回显每一项的实际提取值与期望值。

4. **模拟器同源链路对齐**：
   - 在 `internal/httpapi/rules.go` 的 `handleSimulateRules` 中完整补齐 `vctx.ApplyEnrichers` 与 `rules.CheckInterceptors`；
   - 当触发前置拦截时，模拟器如实呈现熔断状态与留痕，杜绝线上与诊断分叉。

## 代价与后果

- 新增接口间接调用（`interface dispatch`），但经基准测试实测，单条规则纳秒级内存求值延迟基本无感（`85ns ~ 340ns` 量级），换取了完全解耦与可插拔性；
- 树状复杂嵌套组与扁平条件模型保持 100% 向后兼容，无需改动现有数据库与 API 协议；
- 契约由 `internal/rules/pipeline_contract_test.go` 固化保障。
