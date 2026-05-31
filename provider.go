package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProviderID 定义支持的提供商标识
type ProviderID string

const (
	ProviderDeepSeek  ProviderID = "deepseek"
	ProviderAlibaba   ProviderID = "alibaba"
	ProviderXiaomi    ProviderID = "xiaomi"
	ProviderZhipu     ProviderID = "zhipu"
	ProviderBaidu     ProviderID = "baidu"
	ProviderVolcano   ProviderID = "volcano"
	ProviderTencent   ProviderID = "tencent"
	ProviderSilicon   ProviderID = "silicon"
	ProviderKimi      ProviderID = "kimi"
	ProviderMiniMax   ProviderID = "minimax"
	ProviderGoogle    ProviderID = "google"
	ProviderAnthropic ProviderID = "anthropic"
	ProviderCustom    ProviderID = "custom"
)

// ProviderInfo 描述一个 LLM 提供商的元信息
type ProviderInfo struct {
	ID                      ProviderID        // 内部标识
	Name                    string            // 显示名称
	DefaultBaseURL          string            // 默认 API 地址 (OpenAI 兼容, 按量计费)
	DefaultModel            string            // 默认模型
	AnthropicBaseURL        string            // Anthropic 兼容 API 地址 (按量计费)
	AnthropicModel          string            // Anthropic 兼容默认模型
	AnthropicHaikuModel     string            // Anthropic 兼容 Haiku 级模型（快速）
	AnthropicSonnetModel    string            // Anthropic 兼容 Sonnet 级模型（均衡）
	AnthropicOpusModel      string            // Anthropic 兼容 Opus 级模型（最强）
	ClaudeHaikuModel        string            // Claude gateway Haiku 级模型 ID
	ClaudeSonnetModel       string            // Claude gateway Sonnet 级模型 ID
	ClaudeOpusModel         string            // Claude gateway Opus 级模型 ID
	TokenPlanOpenAIBaseURL  string            // Token Plan OpenAI 兼容地址
	TokenPlanAnthropicBaseURL string          // Token Plan Anthropic 兼容地址
	DocsURL                 string            // API 文档地址
	DefaultMappings         map[string]string // Codex 模型 → 提供商模型映射
	HasBalanceAPI           bool              // 是否有公开余额查询接口
	BalanceCheckFn          func(apiKey, baseURL string) (*UsageBalance, error)
	APIType                 APIType           // 该提供商原生支持的 API 格式
	VisionSupported         bool              // Chat Completions 接口是否支持 image_url 图片输入
}

// GetProvider 根据 ID 获取提供商信息；未知 ID 返回 nil
func GetProvider(id ProviderID) *ProviderInfo {
	p, _ := registeredProviders[string(id)]
	return p
}

// GetDefaultProvider 返回默认的 DeepSeek 提供商
func GetDefaultProvider() *ProviderInfo {
	return GetProvider(ProviderDeepSeek)
}

// ClaudeBaseMappings returns model mappings suitable for Claude source profiles.
// It maps standard Claude model IDs to the provider's equivalent tier models.
// Returns nil when the provider has no Anthropic-compatible endpoint.
func (p *ProviderInfo) ClaudeBaseMappings() map[string]string {
	if p == nil {
		return nil
	}
	// Resolve provider-side models per tier
	haiku := p.AnthropicHaikuModel
	if haiku == "" {
		haiku = p.AnthropicModel
	}
	if haiku == "" {
		haiku = p.DefaultModel
	}
	sonnet := p.AnthropicSonnetModel
	if sonnet == "" {
		sonnet = p.AnthropicModel
	}
	if sonnet == "" {
		sonnet = p.DefaultModel
	}
	opus := p.AnthropicOpusModel
	if opus == "" {
		opus = p.AnthropicModel
	}
	if opus == "" {
		opus = p.DefaultModel
	}
	// Resolve Claude-side model IDs
	claudeHaiku := p.ClaudeHaikuModel
	if claudeHaiku == "" {
		claudeHaiku = "claude-haiku-4-5"
	}
	claudeSonnet := p.ClaudeSonnetModel
	if claudeSonnet == "" {
		claudeSonnet = "claude-sonnet-4-6"
	}
	claudeOpus := p.ClaudeOpusModel
	if claudeOpus == "" {
		claudeOpus = "claude-opus-4-7"
	}
	return map[string]string{
		claudeHaiku:  haiku,
		claudeSonnet: sonnet,
		claudeOpus:   opus,
	}
}

// AllProviders 返回所有预置提供商列表（不含 "custom"）
func AllProviders() []ProviderInfo {
	list := make([]ProviderInfo, 0, len(registeredProviders))
	for _, p := range registeredProviders {
		if p.ID != ProviderCustom {
			list = append(list, *p)
		}
	}
	return list
}

// deepseekBalanceCheck 查询 DeepSeek 余额
func deepseekBalanceCheck(apiKey, baseURL string) (*UsageBalance, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	balanceURL := fmt.Sprintf("%s://%s/user/balance", parsed.Scheme, parsed.Host)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, balanceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", upstreamError(resp.StatusCode, body))
	}

	var balanceResp struct {
		IsAvailable  bool `json:"is_available"`
		BalanceInfos []struct {
			Currency        string `json:"currency"`
			TotalBalance    string `json:"total_balance"`
			GrantedBalance  string `json:"granted_balance"`
			ToppedUpBalance string `json:"topped_up_balance"`
		} `json:"balance_infos"`
	}
	if err := json.Unmarshal(body, &balanceResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	result := &UsageBalance{
		AvailableBalance: "",
		TotalBalance:     "",
		Currency:         "",
		IsDepleted:       !balanceResp.IsAvailable,
	}
	if len(balanceResp.BalanceInfos) > 0 {
		info := balanceResp.BalanceInfos[0]
		result.AvailableBalance = info.ToppedUpBalance
		result.TotalBalance = info.TotalBalance
		result.Currency = info.Currency
	}
	return result, nil
}

// registeredProviders 是全局提供商注册表
var registeredProviders = map[string]*ProviderInfo{
	string(ProviderDeepSeek): {
		ID:                   ProviderDeepSeek,
		Name:                 "DeepSeek",
		DefaultBaseURL:       "https://api.deepseek.com/v1",
		DefaultModel:         "deepseek-v4-flash",
		AnthropicBaseURL:     "https://api.deepseek.com/anthropic",
		AnthropicModel:       "deepseek-v4-flash",
		AnthropicHaikuModel:  "deepseek-v4-flash",
		AnthropicSonnetModel: "deepseek-v4-pro",
		AnthropicOpusModel:   "deepseek-v4-pro",
		DocsURL:              "https://api-docs.deepseek.com/",
		HasBalanceAPI:        true,
		BalanceCheckFn:       deepseekBalanceCheck,
		APIType:              APIChatCompletions,
		VisionSupported:      false,
		DefaultMappings:      deepseekDefaultMappings(),
	},
	string(ProviderAlibaba): {
		ID:               ProviderAlibaba,
		Name:             "阿里通义千问",
		DefaultBaseURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
		DefaultModel:     "qwen3.6-plus",
		AnthropicBaseURL: "https://dashscope.aliyuncs.com/api/v2/apps/claude-code-proxy",
		AnthropicModel:   "qwen3.6-plus",
		DocsURL:          "https://help.aliyun.com/zh/model-studio/models",
		HasBalanceAPI:    false,
		BalanceCheckFn:   nil,
		APIType:          APIChatCompletions,
		VisionSupported:  true,
		DefaultMappings:  alibabaDefaultMappings(),
	},
	string(ProviderXiaomi): {
		ID:                        ProviderXiaomi,
		Name:                      "小米 MiMo",
		DefaultBaseURL:            "https://api.xiaomimimo.com/v1",
		DefaultModel:              "mimo-v2.5-pro",
		AnthropicBaseURL:          "https://api.xiaomimimo.com/anthropic",
		AnthropicModel:            "mimo-v2.5-pro",
		TokenPlanOpenAIBaseURL:    "https://token-plan-cn.xiaomimimo.com/v1",
		TokenPlanAnthropicBaseURL: "https://token-plan-cn.xiaomimimo.com/anthropic",
		DocsURL:                   "https://platform.xiaomimimo.com/#/docs/welcome",
		HasBalanceAPI:             false,
		BalanceCheckFn:            nil,
		APIType:                   APIChatCompletions,
		VisionSupported:           false,
		DefaultMappings:           xiaomiDefaultMappings(),
	},
	string(ProviderZhipu): {
		ID:               ProviderZhipu,
		Name:             "智谱 GLM",
		DefaultBaseURL:   "https://open.bigmodel.cn/api/paas/v4",
		DefaultModel:     "glm-4.7-flash",
		AnthropicBaseURL: "https://open.bigmodel.cn/api/anthropic",
		AnthropicModel:   "glm-4.7-flash",
		DocsURL:          "https://docs.bigmodel.cn/",
		HasBalanceAPI:    false,
		BalanceCheckFn:   nil,
		APIType:          APIChatCompletions,
		VisionSupported:  true,
		DefaultMappings:  zhipuDefaultMappings(),
	},
	string(ProviderBaidu): {
		ID:               ProviderBaidu,
		Name:             "百度千帆",
		DefaultBaseURL:   "https://qianfan.baidubce.com/v2",
		DefaultModel:     "ernie-5.1",
		AnthropicBaseURL: "https://qianfan.baidubce.com/anthropic",
		AnthropicModel:   "ernie-5.1",
		DocsURL:          "https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Fm2vrveyu",
		HasBalanceAPI:    false,
		BalanceCheckFn:   nil,
		APIType:          APIChatCompletions,
		VisionSupported:  false,
		DefaultMappings:  baiduDefaultMappings(),
	},
	string(ProviderVolcano): {
		ID:                        ProviderVolcano,
		Name:                      "火山引擎豆包",
		DefaultBaseURL:            "https://ark.cn-beijing.volces.com/api/v3",
		DefaultModel:              "doubao-seed-2-0-lite-260215",
		AnthropicBaseURL:          "https://ark.cn-beijing.volces.com/api/coding",
		AnthropicModel:            "doubao-seed-2-0-lite-260215",
		TokenPlanOpenAIBaseURL:    "https://ark.cn-beijing.volces.com/api/coding/v3",
		TokenPlanAnthropicBaseURL: "https://ark.cn-beijing.volces.com/api/coding",
		DocsURL:                   "https://www.volcengine.com/docs/82379/1330310",
		HasBalanceAPI:             false,
		BalanceCheckFn:            nil,
		APIType:                   APIChatCompletions,
		VisionSupported:           false,
		DefaultMappings:           volcanoDefaultMappings(),
	},
	string(ProviderTencent): {
		ID:                        ProviderTencent,
		Name:                      "腾讯混元",
		DefaultBaseURL:            "https://api.hunyuan.cloud.tencent.com/v1",
		DefaultModel:              "hunyuan-2.0-thinking-20251109",
		AnthropicBaseURL:          "https://api.lkeap.cloud.tencent.com/plan/anthropic",
		AnthropicModel:            "hunyuan-2.0-thinking-20251109",
		TokenPlanOpenAIBaseURL:    "https://api.lkeap.cloud.tencent.com/plan/v3",
		TokenPlanAnthropicBaseURL: "https://api.lkeap.cloud.tencent.com/plan/anthropic",
		DocsURL:                   "https://cloud.tencent.com/document/product/1729/104753",
		HasBalanceAPI:             false,
		BalanceCheckFn:            nil,
		APIType:                   APIChatCompletions,
		VisionSupported:           false,
		DefaultMappings:           tencentDefaultMappings(),
	},
	string(ProviderSilicon): {
		ID:               ProviderSilicon,
		Name:             "硅基流动",
		DefaultBaseURL:   "https://api.siliconflow.cn/v1",
		DefaultModel:     "deepseek-ai/DeepSeek-V4-Flash",
		AnthropicBaseURL: "https://api.siliconflow.cn",
		AnthropicModel:   "Pro/zai-org/GLM-4.7",
		DocsURL:          "https://docs.siliconflow.cn/",
		HasBalanceAPI:    false,
		BalanceCheckFn:   nil,
		APIType:          APIChatCompletions,
		VisionSupported:  true,
		DefaultMappings:  siliconDefaultMappings(),
	},
	string(ProviderKimi): {
		ID:               ProviderKimi,
		Name:             "Kimi",
		DefaultBaseURL:   "https://api.moonshot.cn/v1",
		DefaultModel:     "kimi-k2.6",
		AnthropicBaseURL: "https://api.moonshot.cn/anthropic",
		AnthropicModel:   "kimi-k2.6",
		DocsURL:          "https://platform.moonshot.cn/docs",
		HasBalanceAPI:    false,
		BalanceCheckFn:   nil,
		APIType:          APIChatCompletions,
		VisionSupported:  false,
		DefaultMappings:  kimiDefaultMappings(),
	},
	string(ProviderMiniMax): {
		ID:               ProviderMiniMax,
		Name:             "MiniMax 海螺",
		DefaultBaseURL:   "https://api.minimax.io/v1",
		DefaultModel:     "MiniMax-M2.7",
		AnthropicBaseURL: "https://api.minimaxi.com/anthropic",
		AnthropicModel:   "MiniMax-M2.7",
		DocsURL:          "https://platform.minimax.io/docs",
		HasBalanceAPI:    false,
		BalanceCheckFn:   nil,
		APIType:          APIChatCompletions,
		VisionSupported:  false,
		DefaultMappings:  minimaxDefaultMappings(),
	},
	string(ProviderGoogle): {
		ID:              ProviderGoogle,
		Name:            "Google Gemini",
		DefaultBaseURL:  "https://generativelanguage.googleapis.com/v1beta",
		DefaultModel:    "gemini-2.5-flash",
		DocsURL:         "https://ai.google.dev/gemini-api/docs",
		HasBalanceAPI:   false,
		BalanceCheckFn:  nil,
		APIType:         APIGoogle,
		VisionSupported: true,
		DefaultMappings: googleDefaultMappings(),
	},
	string(ProviderAnthropic): {
		ID:                ProviderAnthropic,
		Name:              "Anthropic Claude",
		DefaultBaseURL:    "https://api.anthropic.com",
		DefaultModel:      "claude-sonnet-4-6",
		AnthropicBaseURL:  "https://api.anthropic.com",
		AnthropicModel:    "claude-sonnet-4-6",
		ClaudeHaikuModel:  "claude-haiku-4-5",
		ClaudeSonnetModel: "claude-sonnet-4-6",
		ClaudeOpusModel:   "claude-opus-4-7",
		DocsURL:           "https://docs.anthropic.com/en/api",
		HasBalanceAPI:     false,
		BalanceCheckFn:    nil,
		APIType:           APIMessages,
		VisionSupported:   true,
		DefaultMappings:   anthropicDefaultMappings(),
	},
}

func deepseekDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "deepseek-v4-pro",
		"gpt-5.4":                "deepseek-v4-pro",
		"gpt-5.4-mini":           "deepseek-v4-flash",
		"gpt-5.3-codex":          "deepseek-v4-pro",
		"gpt-4.1":                "deepseek-v4-flash",
		"gpt-4o":                 "deepseek-v4-flash",
		"gpt-4o-mini":            "deepseek-v4-flash",
		"o4-mini":                "deepseek-v4-flash",
		"codex-auto-review":      "deepseek-v4-flash",
	}
}

func alibabaDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "qwen3.6-max-preview",
		"gpt-5.4":                "qwen3.6-max-preview",
		"gpt-5.4-mini":           "qwen3.6-flash",
		"gpt-5.3-codex":          "qwen3.6-max-preview",
		"gpt-4.1":                "qwen3.6-flash",
		"gpt-4o":                 "qwen3.6-flash",
		"gpt-4o-mini":            "qwen3.6-flash",
		"o4-mini":                "qwq-plus",
		"codex-auto-review":      "qwen3.6-flash",
	}
}

func xiaomiDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "mimo-v2.5-pro",
		"gpt-5.4":                "mimo-v2.5-pro",
		"gpt-5.4-mini":           "mimo-v2-flash",
		"gpt-5.3-codex":          "mimo-v2.5-pro",
		"gpt-4.1":                "mimo-v2-flash",
		"gpt-4o":                 "mimo-v2-flash",
		"gpt-4o-mini":            "mimo-v2-flash",
		"o4-mini":                "mimo-v2-flash",
		"codex-auto-review":      "mimo-v2-flash",
	}
}

func zhipuDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "glm-5",
		"gpt-5.4":                "glm-5",
		"gpt-5.4-mini":           "glm-4.7-flash",
		"gpt-5.3-codex":          "glm-5",
		"gpt-4.1":                "glm-4.7-flash",
		"gpt-4o":                 "glm-4.7-flash",
		"gpt-4o-mini":            "glm-4.7-flash",
		"o4-mini":                "glm-4.7-flash",
		"codex-auto-review":      "glm-4.7-flash",
	}
}

func baiduDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "ernie-5.1",
		"gpt-5.4":                "ernie-5.1",
		"gpt-5.4-mini":           "ernie-4.5-turbo-128k-preview",
		"gpt-5.3-codex":          "ernie-5.1",
		"gpt-4.1":                "ernie-speed-128k",
		"gpt-4o":                 "ernie-speed-128k",
		"gpt-4o-mini":            "ernie-lite-8k",
		"o4-mini":                "ernie-5.0-thinking-preview",
		"codex-auto-review":      "ernie-speed-128k",
	}
}

func volcanoDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "doubao-seed-2-0-pro-260215",
		"gpt-5.4":                "doubao-seed-2-0-pro-260215",
		"gpt-5.4-mini":           "doubao-seed-2-0-lite-260215",
		"gpt-5.3-codex":          "doubao-seed-2-0-code-preview-260215",
		"gpt-4.1":                "doubao-seed-2-0-lite-260215",
		"gpt-4o":                 "doubao-seed-2-0-lite-260215",
		"gpt-4o-mini":            "doubao-seed-2-0-mini-260215",
		"o4-mini":                "doubao-seed-2-0-mini-260215",
		"codex-auto-review":      "doubao-seed-2-0-lite-260215",
	}
}

func tencentDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "hunyuan-2.0-thinking-20251109",
		"gpt-5.4":                "hunyuan-2.0-instruct-20251111",
		"gpt-5.4-mini":           "hunyuan-turbos-latest",
		"gpt-5.3-codex":          "hunyuan-2.0-thinking-20251109",
		"gpt-4.1":                "hunyuan-lite",
		"gpt-4o":                 "hunyuan-lite",
		"gpt-4o-mini":            "hunyuan-lite",
		"o4-mini":                "hunyuan-t1-latest",
		"codex-auto-review":      "hunyuan-lite",
	}
}

func siliconDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "Pro/zai-org/GLM-5.1",
		"gpt-5.4":                "deepseek-ai/DeepSeek-V4-Flash",
		"gpt-5.4-mini":           "Qwen/Qwen3.6-35B-A3B",
		"gpt-5.3-codex":          "deepseek-ai/DeepSeek-V4-Flash",
		"gpt-4.1":                "Qwen/Qwen3.6-27B",
		"gpt-4o":                 "deepseek-ai/DeepSeek-V4-Flash",
		"gpt-4o-mini":            "Qwen/Qwen3.6-35B-A3B",
		"o4-mini":                "deepseek-ai/DeepSeek-V4-Flash",
		"codex-auto-review":      "deepseek-ai/DeepSeek-V4-Flash",
	}
}

func kimiDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":                "kimi-k2.6",
		"gpt-5.4":                "kimi-k2.6",
		"gpt-5.4-mini":           "kimi-k2.6",
		"gpt-5.3-codex":          "kimi-k2.6",
		"gpt-4.1":                "kimi-k2.6",
		"gpt-4o":                 "kimi-k2.6",
		"gpt-4o-mini":            "kimi-k2.6",
		"o4-mini":                "kimi-k2.6-thinking",
		"codex-auto-review":      "kimi-k2.6",
	}
}

func minimaxDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":           "MiniMax-M2.7",
		"gpt-5.4":           "MiniMax-M2.7",
		"gpt-5.4-mini":      "MiniMax-M2.7-highspeed",
		"gpt-5.3-codex":     "MiniMax-M2.7",
		"gpt-4.1":           "MiniMax-M2.5",
		"gpt-4o":            "MiniMax-M2.5",
		"gpt-4o-mini":       "MiniMax-M2.7-highspeed",
		"o4-mini":           "MiniMax-M2.7-highspeed",
		"codex-auto-review": "MiniMax-M2.5",
	}
}

func googleDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":           "gemini-2.5-pro",
		"gpt-5.4":           "gemini-2.5-pro",
		"gpt-5.4-mini":      "gemini-2.5-flash",
		"gpt-5.3-codex":     "gemini-2.5-pro",
		"gpt-4.1":           "gemini-2.5-flash",
		"gpt-4o":            "gemini-2.5-flash",
		"gpt-4o-mini":       "gemini-2.5-flash",
		"o4-mini":           "gemini-2.5-flash-thinking",
		"codex-auto-review": "gemini-2.5-flash",
	}
}

func anthropicDefaultMappings() map[string]string {
	return map[string]string{
		"gpt-5.5":           "claude-opus-4-7",
		"gpt-5.4":           "claude-opus-4-7",
		"gpt-5.4-mini":      "claude-sonnet-4-6",
		"gpt-5.3-codex":     "claude-sonnet-4-6",
		"gpt-4.1":           "claude-sonnet-4-6",
		"gpt-4o":            "claude-sonnet-4-6",
		"gpt-4o-mini":       "claude-haiku-4-5",
		"o4-mini":           "claude-sonnet-4-6",
		"codex-auto-review": "claude-haiku-4-5",
	}
}

// RegisterProvider 允许外部动态注册新提供商（用于扩展）
func RegisterProvider(info *ProviderInfo) {
	if info != nil && info.ID != "" {
		registeredProviders[string(info.ID)] = info
	}
}
