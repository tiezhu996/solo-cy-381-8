-- 002_recurring_plan.sql 周期账单计划模块（参考；实际表结构由后端 GORM AutoMigrate 维护）
-- 周期账单计划表
CREATE TABLE IF NOT EXISTS recurring_plans (
    id                BIGSERIAL PRIMARY KEY,
    group_id          BIGINT       NOT NULL,
    name              VARCHAR(128) NOT NULL,
    amount            DOUBLE PRECISION NOT NULL,
    category          VARCHAR(32)  NOT NULL,
    payer_id          BIGINT       NOT NULL,
    split_type        VARCHAR(16)  NOT NULL,
    day_of_month      INTEGER      NOT NULL,
    status            VARCHAR(16)  NOT NULL DEFAULT 'active',
    last_run_period   VARCHAR(7)   DEFAULT '',
    last_expense_id   BIGINT,
    last_generated_at TIMESTAMPTZ,
    created_by        BIGINT       NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);
-- 周期账单计划参与人表
CREATE TABLE IF NOT EXISTS recurring_plan_shares (
    id         BIGSERIAL PRIMARY KEY,
    plan_id    BIGINT NOT NULL,
    user_id    BIGINT NOT NULL,
    ratio      DOUBLE PRECISION NOT NULL DEFAULT 0,
    amount     DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, user_id)
);
-- 周期账单计划生成记录表：每个执行周期（YYYY-MM）最多一笔，唯一索引兜底幂等
CREATE TABLE IF NOT EXISTS recurring_plan_runs (
    id           BIGSERIAL PRIMARY KEY,
    plan_id      BIGINT     NOT NULL,
    period       VARCHAR(7) NOT NULL,
    expense_id   BIGINT     NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, period)
);
CREATE INDEX IF NOT EXISTS idx_recurring_plans_group ON recurring_plans (group_id);
CREATE INDEX IF NOT EXISTS idx_recurring_plans_status ON recurring_plans (status);
-- 同一群组内计划名称唯一（已移除的计划不参与查重）
CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_group_name ON recurring_plans (group_id, name) WHERE status <> 'removed';
