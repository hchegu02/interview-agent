package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// SchemaValidator 校验 LLM 输出是否符合预期。
// 返回 nil 表示通过；返回 error 时错误消息会被回灌给 LLM 做自纠正。
//
// 设计上故意用 func 而非反射式 JSON Schema 库：
//   - 业务 schema 不复杂（5~10 个字段），手写 validator 比拖一个 schema 库轻
//   - 错误消息可以"指给 LLM 看"——精确写出哪个字段缺/值不对，LLM 更容易自纠
//   - 单测里好造预期错误
type SchemaValidator func(raw []byte) error

// CallWithSchema 是节点调用 LLM 的标准姿势——
// 强制 JSON 输出 + schema 校验 + 自纠正循环。
//
// 流程：
//  1. 第一次调用，要求 ResponseFormat=json_object
//  2. 拿到 content，跑 validator
//  3. 通过 → 返回原始 content（节点自己 unmarshal 到具体结构体）
//  4. 失败 → 把"你刚才返回了 X，校验失败原因是 Y，请按 schema 重写"回灌
//  5. 至多自纠正 maxFixAttempts 次（默认 1），还不行就 ErrSchemaInvalid
//
// 为什么 maxFixAttempts 默认 1：
//   - 如果一次自纠正还不行，多半是 prompt 本身有问题，再多次也救不了
//   - 节点级别还有 graph.WithRetry 兜底，等于"重新走一遍 prompt 链路"，比死循环自纠强
//   - LLM 调用很贵，控制成本
//
// schemaHint 是给 LLM 看的"目标 schema 描述"，可以是 JSON Schema 字符串、
// few-shot 示例或自然语言说明——哪种 LLM 听得懂用哪种。
// 实践经验：手写"字段名 + 类型 + 1 行注释"的列表，比塞 JSON Schema 标准格式有效。
func CallWithSchema(
	ctx context.Context,
	model ChatModel,
	messages []Message,
	opts Options,
	validate SchemaValidator,
	maxFixAttempts int,
) (*Response, error) {
	if maxFixAttempts < 0 {
		maxFixAttempts = 1
	}
	// 强制 JSON 模式。即便厂商默认开 JSON mode 也再加一次保险。
	opts.ResponseFormat = "json_object"

	conv := append([]Message(nil), messages...)
	var lastBad string
	var lastErr error

	// 总尝试 = 首次 + maxFixAttempts 次自纠正
	for attempt := 0; attempt <= maxFixAttempts; attempt++ {
		// 落 trace 日志：demo CLI 跑完后可以 grep event=llm_schema_attempt
		// 数 attempts 总次数；event=llm_schema_validation_failed 数 schema-retry 次数。
		// Debug 级别避免污染 prod 日志；demo CLI 会把 logger 级别打开。
		slog.DebugContext(ctx, "llm schema attempt",
			"event", "llm_schema_attempt",
			"attempt", attempt,
			"max_fix_attempts", maxFixAttempts,
		)
		resp, err := model.Generate(ctx, conv, opts)
		if err != nil {
			// 上游错误（网络/4xx/超时）直接冒泡，不消耗自纠正预算
			return nil, err
		}

		if verr := validate([]byte(resp.Content)); verr == nil {
			return resp, nil
		} else {
			lastBad = resp.Content
			lastErr = verr
			slog.InfoContext(ctx, "llm schema validation failed",
				"event", "llm_schema_validation_failed",
				"attempt", attempt,
				"reason", verr.Error(),
			)
		}

		// 还有自纠正机会 → 回灌错误
		if attempt < maxFixAttempts {
			conv = append(conv,
				Message{Role: "assistant", Content: lastBad},
				Message{Role: "user", Content: buildFixPrompt(lastErr)},
			)
		}
	}

	return nil, fmt.Errorf("%w: %v (last bad output: %s)",
		ErrSchemaInvalid, lastErr, truncate(lastBad, 200))
}

// buildFixPrompt 构造"请按 schema 重写"的回灌消息。
// 措辞参考 OpenAI cookbook 的 self-correction 模式：
//   - 明确告诉 LLM 上次错在哪
//   - 要求只返回 JSON，不要解释
func buildFixPrompt(verr error) string {
	return fmt.Sprintf(
		"你上一次的回答未通过 schema 校验，错误：%s\n请严格按要求重新输出 JSON，"+
			"不要包含任何解释性文字、markdown 代码块或额外字段。",
		verr.Error(),
	)
}

// =============== 常用 validator 工厂 ===============
//
// 节点写自己的 validator 时直接组合下面的小函数，避免重复造轮子。

// ValidateJSON 只校验 raw 是合法 JSON。
// 是最低门槛——ResponseFormat=json_object 已经能保证大部分情况，
// 但流式拼接 / 网关重写时偶尔会出现尾巴截断的情况。
func ValidateJSON(raw []byte) error {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	return nil
}

// ValidateFields 校验 JSON 对象包含所有必需字段。
// 例：ValidateFields([]byte(raw), "action", "reasoning") 要求两字段都存在且非 null。
func ValidateFields(raw []byte, required ...string) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("not a JSON object: %w", err)
	}
	var missing []string
	for _, k := range required {
		v, ok := m[k]
		if !ok || string(v) == "null" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateEnum 校验 raw 反序列化后某字段值在白名单内。
// 用于 action 字段这种 enum 校验。
func ValidateEnum(raw []byte, field string, allowed ...string) error {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("not a JSON object: %w", err)
	}
	v, ok := m[field]
	if !ok {
		return fmt.Errorf("field %q missing", field)
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("field %q is not a string: %v", field, v)
	}
	for _, a := range allowed {
		if s == a {
			return nil
		}
	}
	return fmt.Errorf("field %q = %q, want one of %v", field, s, allowed)
}

// AndValidators 串联多个 validator，遇到第一个错误就返回。
// 方便在节点里组合：AndValidators(ValidateJSON, fieldsCheck, enumCheck)
func AndValidators(vs ...SchemaValidator) SchemaValidator {
	return func(raw []byte) error {
		for _, v := range vs {
			if err := v(raw); err != nil {
				return err
			}
		}
		return nil
	}
}
