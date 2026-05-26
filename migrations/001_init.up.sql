-- =============================================================
-- 001_init.up.sql
--
-- 初始化 interview-agent 的核心表 + pgvector 扩展。
--
-- 设计原则：
--   1. 一次面试会话的可恢复状态全部进 sessions.state_json (jsonb)，
--      不拆 30 张子表。理由：状态机演进期 schema 抖动大，jsonb 让
--      迭代代价 = 改 Go struct + 改一行 SQL，不用写 migration。
--   2. question_bank 是真正"数据库的工作"——固定 schema + 全文/向量
--      检索 + 标签筛选。这里 schema 要稳定。
--   3. instance_id + lease_until 列预留给 Stage 3 的"实例宕机接管"。
--   4. idempotency_keys 给 Stage 3 的节点级幂等用。
--   5. events 表是审计 / 离线分析用，实时 SSE 走 Redis Streams。
-- =============================================================

-- pgvector 扩展，用于题库向量检索（Stage 2）
CREATE EXTENSION IF NOT EXISTS vector;

-- pg_trgm 给后续 question 内容模糊检索预留（暂未使用）
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- -------------------------------------------------------------
-- sessions：面试会话状态
-- -------------------------------------------------------------
-- 为什么 state_json 是 jsonb：
--   - 域模型迭代频繁，强 schema 拖慢迭代
--   - Redis snapshot 已经是 jsonb 格式，PG 持久化共用同一份序列化
--   - 查询热点字段（user_id / status / updated_at）单独提列建索引
--
-- 为什么 id 是 text 而不是 uuid：
--   - 用 ULID（26 字符），时间有序便于按创建顺序扫描
--   - text 比 uuid 检索性能差一点，但可读性优势更值
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS sessions (
    id              text        PRIMARY KEY,
    user_id         text        NOT NULL,
    status          text        NOT NULL CHECK (status IN ('created','running','paused','completed','failed')),
    current_node    text        NOT NULL DEFAULT '',
    state_json      jsonb       NOT NULL DEFAULT '{}'::jsonb,

    -- 实例接管字段（Stage 3）
    -- instance_id = 当前持有该会话的 app 实例
    -- lease_until = 租约到期时间；超期后其他实例可强夺
    instance_id     text,
    lease_until     timestamptz,

    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    -- 会话 TTL，过期由 cron job 清理；面试场景 24h 足够
    expires_at      timestamptz NOT NULL DEFAULT (now() + interval '24 hours')
);

-- 用户面板"我的会话"查询：where user_id = $1 order by updated_at desc
CREATE INDEX IF NOT EXISTS idx_sessions_user_updated
    ON sessions (user_id, updated_at DESC);

-- 接管扫描："找到 lease 过期且仍在运行的会话"
CREATE INDEX IF NOT EXISTS idx_sessions_orphan
    ON sessions (lease_until)
    WHERE status IN ('running','paused');

-- TTL 清理扫描
CREATE INDEX IF NOT EXISTS idx_sessions_expires
    ON sessions (expires_at)
    WHERE status IN ('completed','failed');

-- -------------------------------------------------------------
-- question_bank：题库（含向量）
-- -------------------------------------------------------------
-- vector(1024) 对齐 DashScope text-embedding-v4 默认维度。
-- v4 也支持 768/1536/2048 配置，切换时改维度 + 重建索引。
-- 经典反模式提醒：embedding 模型不可灰度——同一张表里不能混不同维度/不同模型的向量，
-- 会导致检索语义错乱。切换方案见 README.md 第 5 节。
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS question_bank (
    id              text        PRIMARY KEY,
    content         text        NOT NULL,
    tags            text[]      NOT NULL DEFAULT '{}',
    skill_category  text        NOT NULL DEFAULT '',     -- 'go' | 'redis' | 'system-design' ...
    difficulty      smallint    NOT NULL DEFAULT 3 CHECK (difficulty BETWEEN 1 AND 5),
    source          text        NOT NULL DEFAULT 'manual',
    embedding       vector(1024),
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- HNSW 是 pgvector 0.5+ 推荐的近似最近邻索引，建索引慢但查询快、内存友好。
-- vector_cosine_ops = 余弦距离（语义检索默认）。
-- m=16, ef_construction=64 是 pgvector 文档推荐起点，万条量级够用。
CREATE INDEX IF NOT EXISTS idx_question_bank_embedding
    ON question_bank
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- GIN 索引让 tags && ARRAY['go','redis'] 查询走索引而非全表扫
CREATE INDEX IF NOT EXISTS idx_question_bank_tags
    ON question_bank USING gin (tags);

-- 难度 + 类别复合筛选，pgvector 检索前先过滤候选集，性能更好
CREATE INDEX IF NOT EXISTS idx_question_bank_filter
    ON question_bank (skill_category, difficulty);

-- -------------------------------------------------------------
-- idempotency_keys：节点级幂等表（Stage 3）
-- -------------------------------------------------------------
-- 设计：key = sha256(session_id + node + input_hash + prompt_version)[:16]
-- 同一节点用同一输入重放时，直接返回上次结果，避免 LLM 重复扣费。
-- expires_at 触发 TTL 清理，一般 1h 足够覆盖断点恢复窗口。
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key             text        PRIMARY KEY,
    session_id      text        NOT NULL,
    node            text        NOT NULL,
    result_json     jsonb       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL DEFAULT (now() + interval '1 hour')
);

CREATE INDEX IF NOT EXISTS idx_idem_session ON idempotency_keys (session_id);
CREATE INDEX IF NOT EXISTS idx_idem_expires ON idempotency_keys (expires_at);

-- -------------------------------------------------------------
-- events：审计 / 离线分析（Stage 5）
-- -------------------------------------------------------------
-- 实时 SSE 推送走 Redis Streams（低延迟、自动 trim、客户端 Last-Event-ID 重放）。
-- 这里的 events 表是**异步落库**的副本，用于：
--   - 长期审计（用户投诉时回溯 LLM 输出）
--   - 离线训练数据导出
--   - Redis 故障后的兜底
-- 写入路径：Redis Stream consumer -> batch insert，不在主请求链路上。
-- -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS events (
    id              bigserial   PRIMARY KEY,
    session_id      text        NOT NULL,
    event_type      text        NOT NULL,                 -- 'node.start' | 'node.end' | 'token' | 'error' | ...
    node            text        NOT NULL DEFAULT '',
    payload         jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- session 维度回放
CREATE INDEX IF NOT EXISTS idx_events_session_time
    ON events (session_id, created_at);

-- 按事件类型聚合统计
CREATE INDEX IF NOT EXISTS idx_events_type_time
    ON events (event_type, created_at);

-- -------------------------------------------------------------
-- updated_at 触发器：自动维护 sessions.updated_at
-- -------------------------------------------------------------
CREATE OR REPLACE FUNCTION trg_set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS sessions_set_updated_at ON sessions;
CREATE TRIGGER sessions_set_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION trg_set_updated_at();
