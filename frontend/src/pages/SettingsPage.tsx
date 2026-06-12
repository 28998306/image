import {type ReactNode, useEffect, useMemo, useState} from "react";
import {
  GetAppConfig,
  GetImageModels,
  GetVideoModels,
  OpenOutputDir,
  RegConfig,
  RegSetConfig,
  SaveAppConfig,
} from "../../wailsjs/go/main/App";
import type {AppConfig, ImageModelConfig, VideoModelConfig} from "../types";

const PROXY_KEY = "proxy.dynamic_url";

type SettingsTab = "connection" | "image" | "video";

const defaultConfig: AppConfig = {
  provider: "gpt2api",
  baseUrl: "https://www.gpt2api.com/v1",
  apiKey: "",
  defaultModel: "nano-banana-pro",
  defaultImageModel: "nano-banana-pro",
  defaultQuality: "1k",
  defaultSize: "1024x1024",
  defaultVideoModel: "grok-imagine-video",
  defaultDuration: 10,
  defaultRatio: "16:9",
  defaultVideoQuality: "hd",
  asyncImages: true,
  asyncVideos: true,
  callbackUrl: "",
  outputDir: "",
};

const qualityLabels: Record<string, string> = {
  "1k": "1K 快速草图",
  "2k": "2K 交付主力",
  "4k": "4K 海报大图",
};

const tabItems: Array<{id: SettingsTab; label: string; desc: string}> = [
  {id: "connection", label: "接口连接", desc: "API 鉴权与请求策略"},
  {id: "image", label: "图片设置", desc: "默认模型、质量与尺寸"},
  {id: "video", label: "视频设置", desc: "默认模型与生成参数"},
];

function SettingsCard({title, desc, children}: {title: string; desc?: string; children: ReactNode}) {
  return (
    <section className="panel">
      <div className="panel-header">
        <h2 className="section-title">{title}</h2>
        {desc ? <p className="section-desc">{desc}</p> : null}
      </div>
      <div className="p-4">{children}</div>
    </section>
  );
}

export function SettingsPage() {
  const [activeTab, setActiveTab] = useState<SettingsTab>("connection");
  const [config, setConfig] = useState<AppConfig>(defaultConfig);
  const [models, setModels] = useState<ImageModelConfig[]>([]);
  const [videoModels, setVideoModels] = useState<VideoModelConfig[]>([]);
  const [selectedModelId, setSelectedModelId] = useState(defaultConfig.defaultImageModel);
  const [selectedQuality, setSelectedQuality] = useState(defaultConfig.defaultQuality);
  const [showKey, setShowKey] = useState(false);
  const [saveStatus, setSaveStatus] = useState("");
  const [proxyUrl, setProxyUrl] = useState("");

  useEffect(() => {
    GetAppConfig().then((value) => {
      const nextConfig = {...defaultConfig, ...value} as AppConfig;
      setConfig(nextConfig);
      setSelectedModelId(nextConfig.defaultImageModel || nextConfig.defaultModel || defaultConfig.defaultImageModel);
      setSelectedQuality(nextConfig.defaultQuality || defaultConfig.defaultQuality);
    });

    GetImageModels().then((value) => setModels(value as ImageModelConfig[]));
    GetVideoModels().then((value) => setVideoModels(value as VideoModelConfig[]));
    RegConfig()
      .then((cfg) => setProxyUrl((cfg && cfg[PROXY_KEY]) || ""))
      .catch(() => undefined);
  }, []);

  const selectedModel = useMemo(
    () => models.find((model) => model.id === selectedModelId) || models[0],
    [models, selectedModelId],
  );

  const sizeOptions = selectedModel?.sizes?.[selectedQuality] || [];

  const updateConfig = <K extends keyof AppConfig>(key: K, value: AppConfig[K]) => {
    setConfig((current) => ({...current, [key]: value}));
  };

  const selectDefaultModel = (model: ImageModelConfig) => {
    const firstSize = model.sizes?.[selectedQuality]?.[0] || defaultConfig.defaultSize;
    setSelectedModelId(model.id);
    setConfig((current) => ({
      ...current,
      defaultModel: model.id,
      defaultImageModel: model.id,
      defaultSize: firstSize,
    }));
  };

  const selectQuality = (quality: string) => {
    const firstSize = selectedModel?.sizes?.[quality]?.[0] || config.defaultSize;
    setSelectedQuality(quality);
    setConfig((current) => ({
      ...current,
      defaultQuality: quality,
      defaultSize: firstSize,
    }));
  };

  const saveConfig = async () => {
    const ok = await SaveAppConfig({
      ...config,
      provider: "gpt2api",
      defaultModel: config.defaultImageModel,
    });
    try {
      await RegSetConfig({[PROXY_KEY]: proxyUrl.trim()});
    } catch {
      setSaveStatus("配置已保存，但代理网关写入失败");
      return;
    }
    setSaveStatus(ok ? "配置已保存到本机" : "保存失败，请检查配置目录权限");
  };

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 py-3">
        <div>
          <h1 className="app-title">系统设置</h1>
          <p className="mt-0.5 text-sm text-slate-500">接入 gpt2api，配置图片与视频生成的默认参数</p>
        </div>
        <div className="flex items-center gap-2">
          {saveStatus ? <span className="text-xs text-slate-500">{saveStatus}</span> : null}
          <button className="btn-primary" type="button" onClick={saveConfig}>
            保存配置
          </button>
        </div>
      </div>

      <div className="flex min-h-0 flex-1 overflow-hidden">
        <nav className="flex w-[200px] shrink-0 flex-col gap-1 border-r border-slate-200 bg-white p-3">
          {tabItems.map((tab) => (
            <button
              key={tab.id}
              className={`border px-3 py-2.5 text-left transition ${
                activeTab === tab.id
                  ? "border-brand-600 bg-blue-50 text-brand-700"
                  : "border-transparent text-slate-600 hover:border-slate-200 hover:bg-slate-50"
              }`}
              type="button"
              onClick={() => setActiveTab(tab.id)}
            >
              <div className="text-sm">{tab.label}</div>
              <div className="mt-0.5 text-[11px] leading-4 text-slate-500">{tab.desc}</div>
            </button>
          ))}
        </nav>

        <div className="min-h-0 flex-1 overflow-auto p-4">
          {activeTab === "connection" ? (
            <div className="mx-auto flex max-w-4xl flex-col gap-4">
              <SettingsCard title="服务商连接" desc="API Key 仅保存在当前 Windows 用户配置目录">
                <div className="grid gap-4 md:grid-cols-2">
                  <label>
                    <span className="field-label">服务商</span>
                    <input className="field-input bg-slate-50" readOnly value="gpt2api" />
                  </label>

                  <label>
                    <span className="field-label">Base URL</span>
                    <input
                      className="field-input"
                      value={config.baseUrl}
                      onChange={(event) => updateConfig("baseUrl", event.target.value)}
                    />
                  </label>

                  <label className="md:col-span-2">
                    <span className="field-label">API Key</span>
                    <div className="flex gap-2">
                      <input
                        className="field-input"
                        placeholder="sk-..."
                        type={showKey ? "text" : "password"}
                        value={config.apiKey}
                        onChange={(event) => updateConfig("apiKey", event.target.value)}
                      />
                      <button className="btn-secondary shrink-0" type="button" onClick={() => setShowKey((value) => !value)}>
                        {showKey ? "隐藏" : "显示"}
                      </button>
                    </div>
                  </label>
                </div>
              </SettingsCard>

              <SettingsCard title="请求策略" desc="控制异步提交与回调通知">
                <div className="space-y-3">
                  <label className="flex items-start gap-3 border border-slate-200 bg-slate-50 p-3">
                    <input
                      checked={config.asyncImages}
                      className="mt-0.5 accent-brand-600"
                      type="checkbox"
                      onChange={(event) => updateConfig("asyncImages", event.target.checked)}
                    />
                    <div>
                      <div className="text-sm text-slate-800">图片异步生成</div>
                      <div className="mt-0.5 text-xs text-slate-500">默认 async=true，避免长时间阻塞桌面端</div>
                    </div>
                  </label>

                  <label className="flex items-start gap-3 border border-slate-200 bg-slate-50 p-3">
                    <input
                      checked={config.asyncVideos}
                      className="mt-0.5 accent-brand-600"
                      type="checkbox"
                      onChange={(event) => updateConfig("asyncVideos", event.target.checked)}
                    />
                    <div>
                      <div className="text-sm text-slate-800">视频异步生成</div>
                      <div className="mt-0.5 text-xs text-slate-500">默认 async=true，按 retry_after 自动轮询</div>
                    </div>
                  </label>

                  <label>
                    <span className="field-label">Webhook / Callback URL（可选）</span>
                    <input
                      className="field-input"
                      placeholder="https://your-server.com/webhooks/gpt2api"
                      value={config.callbackUrl}
                      onChange={(event) => updateConfig("callbackUrl", event.target.value)}
                    />
                  </label>
                </div>
              </SettingsCard>

              <SettingsCard
                title="公共代理网关"
                desc="全局共享：号池注册、号池生图、刷新有效期、额度查询等所有请求都走该网关"
              >
                <div className="space-y-2">
                  <label>
                    <span className="field-label flex items-center gap-2">
                      动态代理 IP 网关
                      {proxyUrl.trim() ? (
                        <span className="rounded-sm bg-emerald-100 px-1.5 py-0.5 text-[10px] text-emerald-700">已配置</span>
                      ) : (
                        <span className="rounded-sm bg-amber-100 px-1.5 py-0.5 text-[10px] text-amber-700">未配置（直连）</span>
                      )}
                    </span>
                    <input
                      className="field-input"
                      placeholder="http://user:pass@gateway.host:port"
                      value={proxyUrl}
                      onChange={(event) => setProxyUrl(event.target.value)}
                    />
                  </label>
                  <p className="text-[11px] text-slate-500">
                    配置后所有任务统一走该网关，由网关每次连接自动轮换出口 IP；留空则直连本机网络。
                  </p>
                </div>
              </SettingsCard>

              <SettingsCard title="输出目录" desc="生成结果自动保存到本地">
                <div className="flex items-center gap-3">
                  <div className="min-w-0 flex-1 truncate border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-600">
                    {config.outputDir || "默认：用户图片目录 / Web2Img AI Studio"}
                  </div>
                  <button className="btn-secondary shrink-0" type="button" onClick={OpenOutputDir}>
                    打开目录
                  </button>
                </div>
              </SettingsCard>

              <div className="notice-box notice-box-blue">
                图片和视频请求会携带 Authorization Bearer 和 Idempotency-Key。带参考图时走 image / images 兼容入口。
              </div>
            </div>
          ) : null}

          {activeTab === "image" ? (
            <div className="flex min-h-0 flex-col gap-4">
              <div className="panel">
                <div className="panel-header flex items-center justify-between">
                  <div>
                    <h2 className="section-title">默认图片参数</h2>
                    <p className="section-desc">创作工作台打开时自动应用</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="status-chip">{models.length} 个模型</span>
                    <span className="tag min-h-0 text-[11px]">{config.defaultImageModel}</span>
                  </div>
                </div>

                <div className="grid gap-4 p-4 lg:grid-cols-[1fr_300px]">
                  <div className="grid auto-rows-min grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-2">
                    {models.map((model) => (
                      <button
                        key={model.id}
                        className={`border bg-white p-4 text-left transition ${
                          selectedModelId === model.id
                            ? "border-brand-600 bg-blue-50"
                            : "border-slate-200 hover:border-blue-300"
                        }`}
                        type="button"
                        onClick={() => selectDefaultModel(model)}
                      >
                        <div className="flex items-start justify-between gap-2">
                          <div className="min-w-0">
                            <div className="text-sm text-slate-900">{model.name}</div>
                            <div className="mt-0.5 truncate text-[11px] text-slate-500">{model.id}</div>
                          </div>
                          {config.defaultImageModel === model.id ? (
                            <span className="tag min-h-0 shrink-0 text-[11px]">默认</span>
                          ) : null}
                        </div>

                        <p className="mt-2 line-clamp-2 text-xs leading-5 text-slate-600">{model.description}</p>

                        <div className="mt-3 grid grid-cols-3 gap-1.5 text-center text-[11px]">
                          <div className="border border-slate-200 bg-slate-50 px-1 py-1.5">1K · {model.price1k}</div>
                          <div className="border border-slate-200 bg-slate-50 px-1 py-1.5">2K · {model.price2k}</div>
                          <div className="border border-slate-200 bg-slate-50 px-1 py-1.5">4K · {model.price4k}</div>
                        </div>
                      </button>
                    ))}
                  </div>

                  <aside className="border border-slate-200 bg-slate-50 p-4">
                    <h3 className="text-sm text-slate-900">{selectedModel?.name || "尺寸配置"}</h3>
                    <p className="mt-0.5 text-xs text-slate-500">选择质量档与默认输出尺寸</p>

                    <div className="mt-4 grid grid-cols-3 gap-2">
                      {Object.entries(qualityLabels).map(([quality, label]) => (
                        <button
                          key={quality}
                          className={`border px-2 py-2 text-xs ${
                            selectedQuality === quality
                              ? "border-brand-600 bg-blue-50 text-brand-700"
                              : "border-slate-200 bg-white text-slate-600"
                          }`}
                          type="button"
                          onClick={() => selectQuality(quality)}
                        >
                          {quality.toUpperCase()}
                        </button>
                      ))}
                    </div>

                    <p className="mt-2 text-[11px] text-slate-500">{qualityLabels[selectedQuality]}</p>

                    <div className="mt-3 max-h-[360px] space-y-1.5 overflow-auto">
                      {sizeOptions.map((size) => (
                        <button
                          key={size}
                          className={`flex w-full items-center justify-between border px-3 py-2 text-left text-xs ${
                            config.defaultSize === size
                              ? "border-brand-600 bg-blue-50 text-brand-700"
                              : "border-slate-200 bg-white text-slate-700"
                          }`}
                          type="button"
                          onClick={() => updateConfig("defaultSize", size)}
                        >
                          <span>{size}</span>
                          {config.defaultSize === size ? <span>默认</span> : null}
                        </button>
                      ))}
                    </div>
                  </aside>
                </div>
              </div>
            </div>
          ) : null}

          {activeTab === "video" ? (
            <div className="mx-auto flex max-w-4xl flex-col gap-4">
              <SettingsCard title="默认视频参数" desc="创作工作台视频模式打开时自动应用">
                <div className="grid gap-4 md:grid-cols-2">
                  <label>
                    <span className="field-label">默认视频模型</span>
                    <select
                      className="field-input"
                      value={config.defaultVideoModel}
                      onChange={(event) => updateConfig("defaultVideoModel", event.target.value)}
                    >
                      {videoModels.map((model) => (
                        <option key={model.id} value={model.id}>
                          {model.name}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label>
                    <span className="field-label">清晰度</span>
                    <select
                      className="field-input"
                      value={config.defaultVideoQuality}
                      onChange={(event) => updateConfig("defaultVideoQuality", event.target.value)}
                    >
                      <option value="hd">HD (720p)</option>
                      <option value="fullhd">Full HD (1080p)</option>
                    </select>
                  </label>

                  <label>
                    <span className="field-label">时长</span>
                    <select
                      className="field-input"
                      value={config.defaultDuration}
                      onChange={(event) => updateConfig("defaultDuration", Number(event.target.value))}
                    >
                      {[4, 6, 8, 10, 12, 20, 30].map((duration) => (
                        <option key={duration} value={duration}>
                          {duration} 秒
                        </option>
                      ))}
                    </select>
                  </label>

                  <label>
                    <span className="field-label">画面比例</span>
                    <select
                      className="field-input"
                      value={config.defaultRatio}
                      onChange={(event) => updateConfig("defaultRatio", event.target.value)}
                    >
                      {["16:9", "9:16", "1:1"].map((ratio) => (
                        <option key={ratio} value={ratio}>
                          {ratio}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
              </SettingsCard>

              <SettingsCard title="可用视频模型" desc="gpt2api 当前开放的视频生成模型">
                <div className="grid gap-3 sm:grid-cols-2">
                  {videoModels.map((model) => (
                    <div
                      key={model.id}
                      className={`border p-4 ${
                        config.defaultVideoModel === model.id
                          ? "border-brand-600 bg-blue-50"
                          : "border-slate-200 bg-white"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="text-sm text-slate-900">{model.name}</div>
                        {config.defaultVideoModel === model.id ? (
                          <span className="tag min-h-0 text-[11px]">默认</span>
                        ) : (
                          <button
                            className="btn-text text-[11px]"
                            type="button"
                            onClick={() => updateConfig("defaultVideoModel", model.id)}
                          >
                            设为默认
                          </button>
                        )}
                      </div>
                      <div className="mt-0.5 text-[11px] text-slate-500">{model.id}</div>
                      <p className="mt-2 text-xs leading-5 text-slate-600">{model.description}</p>
                      <div className="mt-3 border border-slate-200 bg-slate-50 px-2 py-1.5 text-center text-[11px] text-slate-600">
                        {model.pricing}
                      </div>
                    </div>
                  ))}
                </div>
              </SettingsCard>

              <div className="notice-box">
                文生视频不带参考图；图生视频上传 image / images[]。20s / 30s 长视频由后端自动拼接返回。
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </main>
  );
}
