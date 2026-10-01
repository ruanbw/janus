-- 0014_rule_expression.sql — 规则表达式扩展 (Expr)
--
-- 支持高级用户使用 Expr 语言编写自定义求值逻辑 (Tier-2 规则)。
-- rule_type: 'visual' (默认可视化条件树) | 'expression' (Expr 表达式)
-- expression: 当 rule_type = 'expression' 时存放表达式源码

ALTER TABLE rules
    ADD COLUMN rule_type VARCHAR(20) NOT NULL DEFAULT 'visual',
    ADD COLUMN expression TEXT NOT NULL DEFAULT '';
