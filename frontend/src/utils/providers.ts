export interface ProviderPreset {
  id: string
  label: string
  defaultBaseURL: string
  defaultModel: string
  anthropicBaseURL?: string
  anthropicModel?: string
  tokenPlanOpenAIBaseURL?: string
  tokenPlanAnthropicBaseURL?: string
  docsURL: string
  placeholderApiKey: string
  placeholderModel: string
  apiType: 'chat_completions' | 'responses' | 'messages' | 'google'
  visionSupported: boolean  // whether the Chat Completions endpoint supports image_url
}

// ClaudeBaseMappings generates Claude-specific model mappings for a provider.
// It maps standard Claude model IDs to the provider's equivalent models.
export function getClaudeBaseMappings(preset: ProviderPreset): Record<string, string> {
  const target = preset.anthropicModel || preset.defaultModel
  // Anthropic provider maps Claude IDs to themselves.
  if (preset.id === 'anthropic') {
    return {
      'claude-opus-4-7': 'claude-opus-4-7',
      'claude-sonnet-4-6': 'claude-sonnet-4-6',
      'claude-haiku-4-5': 'claude-haiku-4-5',
    }
  }
  return {
    'claude-opus-4-7': target,
    'claude-sonnet-4-6': target,
    'claude-haiku-4-5': target,
  }
}

export const BILLING_MODE_LABELS: Record<string, string> = {
  paygo: '按量计费',
  tokenplan: 'Token Plan',
}

export const API_TYPE_LABELS: Record<string, string> = {
  chat_completions: 'Chat Completions (OpenAI)',
  responses: 'Responses (Codex)',
  messages: 'Messages (Anthropic)',
  google: 'Google (Gemini)',
}

export const PROVIDER_PRESETS: ProviderPreset[] = [
  {
    id: 'deepseek',
    label: 'DeepSeek',
    defaultBaseURL: 'https://api.deepseek.com/v1',
    defaultModel: 'deepseek-v4-flash',
    anthropicBaseURL: 'https://api.deepseek.com/anthropic',
    anthropicModel: 'deepseek-v4-flash',
    docsURL: 'https://api-docs.deepseek.com/',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'deepseek-v4-flash',
    apiType: 'chat_completions',
    visionSupported: false,
  },
  {
    id: 'alibaba',
    label: '阿里通义千问',
    defaultBaseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    defaultModel: 'qwen3.6-plus',
    anthropicBaseURL: 'https://dashscope.aliyuncs.com/api/v2/apps/claude-code-proxy',
    anthropicModel: 'qwen3.6-plus',
    docsURL: 'https://help.aliyun.com/zh/model-studio/models',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'qwen3.6-plus',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'xiaomi',
    label: '小米 MiMo',
    defaultBaseURL: 'https://api.xiaomimimo.com/v1',
    defaultModel: 'mimo-v2.5-pro',
    anthropicBaseURL: 'https://api.xiaomimimo.com/anthropic',
    anthropicModel: 'mimo-v2.5-pro',
    tokenPlanOpenAIBaseURL: 'https://token-plan-cn.xiaomimimo.com/v1',
    tokenPlanAnthropicBaseURL: 'https://token-plan-cn.xiaomimimo.com/anthropic',
    docsURL: 'https://platform.xiaomimimo.com/#/docs/welcome',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'mimo-v2.5-pro',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'zhipu',
    label: '智谱 GLM',
    defaultBaseURL: 'https://open.bigmodel.cn/api/paas/v4',
    defaultModel: 'glm-4.7-flash',
    anthropicBaseURL: 'https://open.bigmodel.cn/api/anthropic',
    anthropicModel: 'glm-4.7-flash',
    docsURL: 'https://docs.bigmodel.cn/',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'glm-4.7-flash',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'baidu',
    label: '百度千帆',
    defaultBaseURL: 'https://qianfan.baidubce.com/v2',
    defaultModel: 'ernie-5.1',
    anthropicBaseURL: 'https://qianfan.baidubce.com/anthropic',
    anthropicModel: 'ernie-5.1',
    docsURL: 'https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Fm2vrveyu',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'ernie-5.1',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'volcano',
    label: '火山引擎豆包',
    defaultBaseURL: 'https://ark.cn-beijing.volces.com/api/v3',
    defaultModel: 'doubao-seed-2-0-lite-260215',
    anthropicBaseURL: 'https://ark.cn-beijing.volces.com/api/coding',
    anthropicModel: 'doubao-seed-2-0-lite-260215',
    tokenPlanOpenAIBaseURL: 'https://ark.cn-beijing.volces.com/api/coding/v3',
    tokenPlanAnthropicBaseURL: 'https://ark.cn-beijing.volces.com/api/coding',
    docsURL: 'https://www.volcengine.com/docs/82379/1330310',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'doubao-seed-2-0-lite-260215',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'tencent',
    label: '腾讯混元',
    defaultBaseURL: 'https://api.hunyuan.cloud.tencent.com/v1',
    defaultModel: 'hunyuan-2.0-thinking-20251109',
    anthropicBaseURL: 'https://api.lkeap.cloud.tencent.com/plan/anthropic',
    anthropicModel: 'hunyuan-2.0-thinking-20251109',
    tokenPlanOpenAIBaseURL: 'https://api.lkeap.cloud.tencent.com/plan/v3',
    tokenPlanAnthropicBaseURL: 'https://api.lkeap.cloud.tencent.com/plan/anthropic',
    docsURL: 'https://cloud.tencent.com/document/product/1729/104753',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'hunyuan-2.0-thinking-20251109',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'silicon',
    label: '硅基流动',
    defaultBaseURL: 'https://api.siliconflow.cn/v1',
    defaultModel: 'deepseek-ai/DeepSeek-V4-Flash',
    anthropicBaseURL: 'https://api.siliconflow.cn',
    anthropicModel: 'Pro/zai-org/GLM-4.7',
    docsURL: 'https://docs.siliconflow.cn/',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'deepseek-ai/DeepSeek-V4-Flash',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'kimi',
    label: 'Kimi',
    defaultBaseURL: 'https://api.moonshot.cn/v1',
    defaultModel: 'kimi-k2.6',
    anthropicBaseURL: 'https://api.moonshot.cn/anthropic',
    anthropicModel: 'kimi-k2.6',
    docsURL: 'https://platform.moonshot.cn/docs',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'kimi-k2.6',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'minimax',
    label: 'MiniMax 海螺',
    defaultBaseURL: 'https://api.minimax.io/v1',
    defaultModel: 'MiniMax-M2.7',
    anthropicBaseURL: 'https://api.minimaxi.com/anthropic',
    anthropicModel: 'MiniMax-M2.7',
    docsURL: 'https://platform.minimax.io/docs',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'MiniMax-M2.7',
    apiType: 'chat_completions',
    visionSupported: true,
  },
  {
    id: 'google',
    label: 'Google Gemini',
    defaultBaseURL: 'https://generativelanguage.googleapis.com/v1beta',
    defaultModel: 'gemini-2.5-flash',
    docsURL: 'https://ai.google.dev/gemini-api/docs',
    placeholderApiKey: 'AIza...',
    placeholderModel: 'gemini-2.5-flash',
    apiType: 'google',
    visionSupported: true,
  },
  {
    id: 'anthropic',
    label: 'Anthropic Claude',
    defaultBaseURL: 'https://api.anthropic.com',
    defaultModel: 'claude-sonnet-4-6',
    anthropicBaseURL: 'https://api.anthropic.com',
    anthropicModel: 'claude-sonnet-4-6',
    docsURL: 'https://docs.anthropic.com/en/api',
    placeholderApiKey: 'sk-ant-...',
    placeholderModel: 'claude-sonnet-4-6',
    apiType: 'messages',
    visionSupported: true,
  },
  {
    id: 'custom',
    label: '自定义',
    defaultBaseURL: '',
    defaultModel: '',
    docsURL: '',
    placeholderApiKey: 'sk-...',
    placeholderModel: 'gpt-4o',
    apiType: 'chat_completions',
    visionSupported: false,
  },
]

export function getProviderPreset(id: string): ProviderPreset | undefined {
  return PROVIDER_PRESETS.find((p) => p.id === id)
}

export function getDefaultProviderPreset(): ProviderPreset {
  return PROVIDER_PRESETS[0]
}
