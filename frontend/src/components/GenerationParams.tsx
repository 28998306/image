import type {GenerationMode, GenerationParams as GenerationParamsType, ImageModelConfig, VideoModelConfig} from "../types";

interface GenerationParamsProps {
  mode: GenerationMode;
  params: GenerationParamsType;
  imageModels: ImageModelConfig[];
  videoModels: VideoModelConfig[];
  isBusy: boolean;
  prompt: string;
  onChange: (params: GenerationParamsType) => void;
  onPromptChange: (value: string) => void;
  onGenerate: () => void;
}

const qualityOptions = [
  {value: "1k", label: "1K 快速草图"},
  {value: "2k", label: "2K 交付主力"},
  {value: "4k", label: "4K 海报大图"},
];

const durationOptions = [4, 6, 8, 10, 12, 20, 30];
const ratioOptions = ["16:9", "9:16", "1:1"];
const videoQualityOptions = [
  {value: "hd", label: "HD 720p"},
  {value: "fullhd", label: "Full HD 1080p"},
];

function gcd(a: number, b: number): number {
  return b === 0 ? a : gcd(b, a % b);
}

function getRatioLabel(size: string) {
  const [width, height] = size.split("x").map((value) => Number(value));
  if (!width || !height) {
    return "自定义";
  }
  const divisor = gcd(width, height);
  return `${width / divisor}:${height / divisor}`;
}

export function GenerationParams({
  mode,
  params,
  imageModels,
  videoModels,
  isBusy,
  prompt,
  onChange,
  onPromptChange,
  onGenerate,
}: GenerationParamsProps) {
  const selectedModel = imageModels.find((model) => model.id === params.model) || imageModels[0];
  const selectedVideoModel = videoModels.find((model) => model.id === params.videoModel) || videoModels[0];
  const sizeOptions = selectedModel?.sizes?.[params.quality] || [];

  const updateParam = <K extends keyof GenerationParamsType>(key: K, value: GenerationParamsType[K]) => {
    onChange({...params, [key]: value});
  };

  const updateModel = (modelId: string) => {
    const nextModel = imageModels.find((model) => model.id === modelId);
    const nextSize = nextModel?.sizes?.[params.quality]?.[0] || params.size;
    onChange({...params, model: modelId, size: nextSize});
  };

  const updateQuality = (quality: string) => {
    const nextSize = selectedModel?.sizes?.[quality]?.[0] || params.size;
    onChange({...params, quality, size: nextSize});
  };

  return (
    <aside className="panel flex min-h-0 flex-1 flex-col">
      <div className="panel-header">
        <h2 className="section-title">生成参数</h2>
        <p className="section-desc">按 gpt2api 图片接口真实字段设置</p>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3">
        {mode === "video" ? (
          <>
            <label>
              <span className="field-label">视频模型</span>
              <select className="field-input" value={params.videoModel} onChange={(event) => updateParam("videoModel", event.target.value)}>
                {videoModels.map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.name}
                  </option>
                ))}
              </select>
              {selectedVideoModel ? <div className="mt-2 text-[11px] leading-4 text-slate-500">{selectedVideoModel.pricing}</div> : null}
            </label>

            <div className="grid grid-cols-2 gap-3">
              <label>
                <span className="field-label">时长</span>
                <select className="field-input" value={params.duration} onChange={(event) => updateParam("duration", Number(event.target.value))}>
                  {durationOptions.map((duration) => (
                    <option key={duration} value={duration}>
                      {duration} 秒
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span className="field-label">比例</span>
                <select className="field-input" value={params.ratio} onChange={(event) => updateParam("ratio", event.target.value)}>
                  {ratioOptions.map((ratio) => (
                    <option key={ratio} value={ratio}>
                      {ratio}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <label>
              <span className="field-label">清晰度</span>
              <select className="field-input" value={params.videoQuality} onChange={(event) => updateParam("videoQuality", event.target.value)}>
                {videoQualityOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
          </>
        ) : (
          <>
            <label>
              <span className="field-label">模型</span>
              <select className="field-input" value={params.model} onChange={(event) => updateModel(event.target.value)}>
                {imageModels.map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.name}
                  </option>
                ))}
              </select>
              {selectedModel ? (
                <div className="mt-2 text-[11px] leading-4 text-slate-500">
                  {selectedModel.id} · 1K {selectedModel.price1k} 点 / 2K {selectedModel.price2k} 点 / 4K {selectedModel.price4k} 点
                </div>
              ) : null}
            </label>

            <label>
              <span className="field-label">图片尺寸</span>
              <select className="field-input" value={params.size} onChange={(event) => updateParam("size", event.target.value)}>
                {sizeOptions.map((size) => (
                  <option key={size} value={size}>
                    {size} · {getRatioLabel(size)}
                  </option>
                ))}
              </select>
              <div className="mt-2 text-[11px] text-slate-500">当前比例：{getRatioLabel(params.size)}</div>
            </label>

            <label>
              <span className="field-label">质量档</span>
              <select className="field-input" value={params.quality} onChange={(event) => updateQuality(event.target.value)}>
                {qualityOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>

            <label>
              <span className="field-label">出图数量</span>
              <input className="field-input" min={1} max={4} type="number" value={params.count} onChange={(event) => updateParam("count", Number(event.target.value))} />
            </label>
          </>
        )}

        <label className="flex min-h-[220px] flex-1 flex-col">
          <span className="field-label">提示词</span>
          <textarea
            className="field-input min-h-[220px] flex-1 resize-none"
            placeholder="请输入你想生成的画面描述"
            value={prompt}
            onChange={(event) => onPromptChange(event.target.value)}
          />
        </label>
      </div>

      <div className="border-t border-slate-200 p-3">
        <button className="btn-primary w-full" disabled={isBusy} type="button" onClick={onGenerate}>
          {isBusy ? "生成中..." : "开始生成"}
        </button>
      </div>
    </aside>
  );
}
