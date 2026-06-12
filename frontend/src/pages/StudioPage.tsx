import {useEffect, useState} from "react";
import {
  GenerateImageToImage,
  GenerateTextToImage,
  GenerateVideo,
  GetAppConfig,
  GetImageModels,
  GetVideoModels,
  PollImageTask,
  PollVideoTask,
  SaveHistory,
  SaveRemoteFile,
} from "../../wailsjs/go/main/App";
import {GenerationParams} from "../components/GenerationParams";
import {GenerationQueue} from "../components/GenerationQueue";
import {ImageInputPanel} from "../components/ImageInputPanel";
import {ResultGallery} from "../components/ResultGallery";
import type {GalleryImage, GenerationMode, GenerationParams as GenerationParamsType, ImageModelConfig, QueueItem, VideoModelConfig} from "../types";
import type {main} from "../../wailsjs/go/models";

const modeOptions: Array<{id: GenerationMode; label: string}> = [
  {id: "text", label: "文生图"},
  {id: "image", label: "图生图"},
  {id: "video", label: "视频生成"},
];

interface StudioPageProps {
  onStatsUpdate: () => void;
}

export function StudioPage({onStatsUpdate}: StudioPageProps) {
  const [mode, setMode] = useState<GenerationMode>("text");
  const [prompt, setPrompt] = useState("");
  const [selectedImageId, setSelectedImageId] = useState("");
  const [galleryImages, setGalleryImages] = useState<GalleryImage[]>([]);
  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [imageModels, setImageModels] = useState<ImageModelConfig[]>([]);
  const [videoModels, setVideoModels] = useState<VideoModelConfig[]>([]);
  const [referenceImages, setReferenceImages] = useState<string[]>([]);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [params, setParams] = useState<GenerationParamsType>({
    model: "nano-banana-pro",
    quality: "1k",
    size: "1024x1024",
    count: 4,
    videoModel: "grok-imagine-video",
    duration: 10,
    ratio: "16:9",
    videoQuality: "hd",
  });

  useEffect(() => {
    GetImageModels().then((models) => {
      const nextModels = models as ImageModelConfig[];
      setImageModels(nextModels);

      GetAppConfig().then((config) => {
        const modelId = config.defaultImageModel || config.defaultModel || nextModels[0]?.id || "nano-banana-pro";
        const quality = config.defaultQuality || "1k";
        const model = nextModels.find((item) => item.id === modelId) || nextModels[0];
        const configuredSize = config.defaultSize;
        const availableSizes = model?.sizes?.[quality] || [];
        const size = availableSizes.includes(configuredSize) ? configuredSize : availableSizes[0] || "1024x1024";

        setParams((current) => ({
          ...current,
          model: modelId,
          quality,
          size,
          videoModel: config.defaultVideoModel || "grok-imagine-video",
          duration: config.defaultDuration || 10,
          ratio: config.defaultRatio || "16:9",
          videoQuality: config.defaultVideoQuality || "hd",
        }));
      });
    });
    GetVideoModels().then((models) => setVideoModels(models as VideoModelConfig[]));
  }, []);

  const updateQueueItem = (id: string, patch: Partial<QueueItem>) => {
    setQueue((currentQueue) => currentQueue.map((item) => (item.id === id ? {...item, ...patch} : item)));
  };

  const appendGeneratedMedia = (urls: string[], sourcePrompt: string, sourceMode: GenerationMode, covers: string[] = []) => {
    if (urls.length === 0) {
      return;
    }

    const createdAt = new Date().toLocaleTimeString("zh-CN", {hour: "2-digit", minute: "2-digit"});
    const nextImages = urls.map((url, index) => ({
      id: `img-${Date.now()}-${index}`,
      title: `生成结果 ${index + 1}`,
      mode: sourceMode,
      prompt: sourcePrompt,
      createdAt,
      size: params.size,
      palette: "from-slate-100 via-blue-50 to-slate-200",
      imageUrl: sourceMode === "video" ? covers[index] || "" : url,
      videoUrl: sourceMode === "video" ? url : "",
      coverUrl: covers[index] || "",
    }));

    setGalleryImages((currentImages) => [...nextImages, ...currentImages]);
    setSelectedImageId(nextImages[0].id);

    nextImages.forEach(async (image) => {
      const assetUrl = image.videoUrl || image.imageUrl || "";
      const localPath = assetUrl ? await SaveRemoteFile(assetUrl, image.title) : "";
      if (localPath) {
        setGalleryImages((currentImages) =>
          currentImages.map((currentImage) => (currentImage.id === image.id ? {...currentImage, localPath} : currentImage)),
        );
      }

      SaveHistory({
        id: image.id,
        title: image.title,
        mode: image.mode,
        prompt: image.prompt,
        model: sourceMode === "video" ? params.videoModel : params.model,
        quality: sourceMode === "video" ? params.videoQuality : params.quality,
        size: sourceMode === "video" ? `${params.duration}s ${params.ratio}` : image.size,
        imageUrl: image.imageUrl || "",
        videoUrl: image.videoUrl || "",
        coverUrl: image.coverUrl || "",
        localPath,
        createdAt: new Date().toISOString(),
      });
      onStatsUpdate();
    });
  };

  const pollMediaTask = async (taskId: string, queueId: string, sourcePrompt: string, sourceMode: GenerationMode) => {
    let retryAfter = 2;

    for (let attempt = 0; attempt < 120; attempt += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, retryAfter * 1000));
      const result = sourceMode === "video" ? await PollVideoTask(taskId) : await PollImageTask(taskId);
      retryAfter = Math.max(result.retryAfter || 2, 1);

      if (result.status === "succeeded") {
        updateQueueItem(queueId, {status: "done", progress: 100, message: "生成完成"});
        appendGeneratedMedia(sourceMode === "video" ? result.videoUrls || [] : result.imageUrls || [], sourcePrompt, sourceMode, result.coverUrls || []);
        return;
      }

      if (result.status === "failed") {
        updateQueueItem(queueId, {status: "failed", progress: result.progress || 0, message: result.message});
        return;
      }

      updateQueueItem(queueId, {
        status: result.status === "queued" ? "waiting" : "running",
        progress: result.progress || 5,
        message: result.message,
      });
    }

    updateQueueItem(queueId, {status: "failed", message: "轮询超时，请稍后在任务列表检查结果"});
  };

  const submitGeneration = (request: main.GenerationRequest) => {
    if (mode === "video") {
      return GenerateVideo(request);
    }
    if (mode === "image") {
      return GenerateImageToImage(request);
    }
    return GenerateTextToImage(request);
  };

  const handleGenerate = async () => {
    if (isSubmitting) {
      return;
    }
    if (!prompt.trim()) {
      return;
    }
    if (mode === "image" && referenceImages.length === 0) {
      const queueId = `job-${Date.now()}`;
      const failedJob: QueueItem = {
        id: queueId,
        title: "缺少参考图",
        status: "failed",
        progress: 0,
        mode,
        message: "图生图需要先上传参考图",
      };
      setQueue((currentQueue) => [
        failedJob,
        ...currentQueue,
      ].slice(0, 3));
      return;
    }

    setIsSubmitting(true);
    const queueId = `job-${Date.now()}`;
    const nextJob: QueueItem = {
      id: queueId,
      title: modeOptions.find((option) => option.id === mode)?.label || "生成任务",
      status: "running",
      progress: 1,
      mode,
      message: "正在提交到 gpt2api",
    };

    setQueue((currentQueue) => [nextJob, ...currentQueue].slice(0, 3));

    try {
      const result = await submitGeneration({
        mode,
        prompt,
        model: mode === "video" ? params.videoModel : params.model,
        quality: mode === "video" ? params.videoQuality : params.quality,
        size: params.size,
        count: params.count,
        duration: params.duration,
        ratio: params.ratio,
        imagePath: referenceImages[0] || "",
        imagePaths: referenceImages,
      } as main.GenerationRequest);

      if (result.status === "failed") {
        updateQueueItem(queueId, {status: "failed", progress: 0, message: result.message});
        return;
      }

      if (result.status === "succeeded") {
        updateQueueItem(queueId, {status: "done", progress: 100, message: "生成完成"});
        appendGeneratedMedia(mode === "video" ? result.videoUrls || [] : result.imageUrls || [], prompt, mode, result.coverUrls || []);
        return;
      }

      updateQueueItem(queueId, {
        id: result.jobId || queueId,
        status: result.status === "queued" ? "waiting" : "running",
        progress: result.progress || 5,
        message: result.message,
      });
      void pollMediaTask(result.jobId, result.jobId || queueId, prompt, mode);
    } catch (error) {
      updateQueueItem(queueId, {status: "failed", progress: 0, message: error instanceof Error ? error.message : "生成失败"});
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <main className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-hidden p-4">
        <div className="flex h-full min-h-0 gap-3">
          <div className="flex min-w-0 flex-1 flex-col gap-3">
            <section className="panel p-2">
              <div className="grid grid-cols-3 gap-2">
                {modeOptions.map((option) => (
                  <button
                    key={option.id}
                    className={`border px-3 py-2 text-center transition ${
                      mode === option.id
                        ? "border-brand-600 bg-blue-50"
                        : "border-slate-200 bg-white hover:border-blue-300"
                    }`}
                    type="button"
                    onClick={() => setMode(option.id)}
                  >
                    <div className="text-sm text-slate-900">{option.label}</div>
                  </button>
                ))}
              </div>
            </section>

            <div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_340px] gap-3">
              <ResultGallery images={galleryImages} selectedId={selectedImageId} onSelect={setSelectedImageId} />

              <div className="flex min-h-0 flex-col gap-3">
                <ImageInputPanel
                  mode={mode}
                  referenceImages={referenceImages}
                  onReferenceImagesChange={setReferenceImages}
                />
                <GenerationParams
                  imageModels={imageModels}
                  videoModels={videoModels}
                  isBusy={isSubmitting}
                  mode={mode}
                  params={params}
                  prompt={prompt}
                  onChange={setParams}
                  onGenerate={handleGenerate}
                  onPromptChange={setPrompt}
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <GenerationQueue queue={queue} />
    </main>
  );
}
