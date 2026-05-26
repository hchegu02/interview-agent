-- seed_question_bank.sql
--
-- 演示用题库种子。embedding 留 NULL，由 Stage 2 的 reindex 工具
-- 读出 content 走 Embedder 计算后 UPDATE 回填。
--
-- 这种"内容先入库、向量异步补"的模式比"必须先算向量再入库"
-- 更适合迭代——切换 embedding 模型时只重跑 reindex，不重建表。

INSERT INTO question_bank (id, content, tags, skill_category, difficulty, source) VALUES
('q-go-001', '请讲讲 Go 的 GMP 调度模型，G/M/P 各自的职责和数量关系。',
    ARRAY['go','runtime','并发'], 'go', 3, 'manual'),

('q-go-002', '为什么 Go 的 map 不是并发安全的？sync.Map 在什么场景下比 mutex+map 更优？',
    ARRAY['go','并发','sync'], 'go', 3, 'manual'),

('q-go-003', 'channel 关闭后再 send 会怎样？再 recv 会怎样？为什么这样设计？',
    ARRAY['go','channel'], 'go', 2, 'manual'),

('q-go-004', 'defer 的执行时机和参数求值时机分别是什么？讲一个常见的 defer 坑。',
    ARRAY['go','defer'], 'go', 2, 'manual'),

('q-go-005', 'GC 触发的几种条件？如何排查一次 GC 暂停过长的问题？',
    ARRAY['go','gc','性能'], 'go', 4, 'manual'),

('q-redis-001', '为什么 Redis 6 之前是单线程模型？6.0 之后的多线程是怎么参与的？',
    ARRAY['redis','架构'], 'redis', 3, 'manual'),

('q-redis-002', 'Redis 持久化 RDB 和 AOF 的区别？生产环境怎么选？',
    ARRAY['redis','持久化'], 'redis', 3, 'manual'),

('q-redis-003', '用 Redis 实现分布式锁需要注意哪些点？SET NX PX + Lua 释放为什么？',
    ARRAY['redis','分布式锁','并发'], 'redis', 4, 'manual'),

('q-redis-004', '缓存穿透、击穿、雪崩分别是什么？分别怎么防？',
    ARRAY['redis','缓存'], 'redis', 3, 'manual'),

('q-sys-001', '设计一个支持百万 QPS 的短链系统，从存储到缓存到限流讲一遍。',
    ARRAY['system-design','缓存','限流'], 'system-design', 5, 'manual'),

('q-sys-002', '一个支付回调接口需要保证幂等，至少给出三种实现方案并对比。',
    ARRAY['system-design','幂等'], 'system-design', 4, 'manual'),

('q-pg-001', 'PostgreSQL 的 MVCC 是怎么实现的？VACUUM 解决什么问题？',
    ARRAY['postgres','存储'], 'postgres', 4, 'manual'),

('q-pg-002', 'B-tree 和 GIN 索引分别适合什么查询场景？',
    ARRAY['postgres','索引'], 'postgres', 3, 'manual'),

('q-net-001', 'HTTP/1.1 keep-alive 和 HTTP/2 多路复用解决了什么不同的问题？',
    ARRAY['网络','http'], 'network', 3, 'manual'),

('q-net-002', 'TCP 三次握手为什么是三次不是两次？四次挥手最后那个 TIME_WAIT 的作用？',
    ARRAY['网络','tcp'], 'network', 3, 'manual')
ON CONFLICT (id) DO NOTHING;
