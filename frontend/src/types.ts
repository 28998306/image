export type GenerationMode = "text" | "image" | "video";

export type QueueStatus = "running" | "done" | "failed" | "waiting";

export interface GenerationParams {
  model: string;
  quality: string;
  size: string;
  count: number;
  videoModel: string;
  duration: number;
  ratio: string;
  videoQuality: string;
}

export interface GalleryImage {
  id: string;
  title: string;
  mode: GenerationMode;
  prompt: string;
  createdAt: string;
  size: string;
  palette: string;
  imageUrl?: string;
  videoUrl?: string;
  coverUrl?: string;
  localPath?: string;
}

export interface HistoryItem {
  id: string;
  title: string;
  mode: GenerationMode;
  prompt: string;
  model: string;
  quality: string;
  size: string;
  imageUrl: string;
  videoUrl: string;
  coverUrl: string;
  localPath: string;
  createdAt: string;
}

export interface VideoModelConfig {
  id: string;
  name: string;
  description: string;
  pricing: string;
}

export interface QueueItem {
  id: string;
  title: string;
  status: QueueStatus;
  progress: number;
  mode: GenerationMode;
  message?: string;
}

export interface ImageModelConfig {
  id: string;
  name: string;
  price1k: string;
  price2k: string;
  price4k: string;
  description: string;
  sizes: Record<string, string[]>;
}

export interface AppConfig {
  provider: string;
  baseUrl: string;
  apiKey: string;
  defaultModel: string;
  defaultImageModel: string;
  defaultQuality: string;
  defaultSize: string;
  defaultVideoModel: string;
  defaultDuration: number;
  defaultRatio: string;
  defaultVideoQuality: string;
  asyncImages: boolean;
  asyncVideos: boolean;
  callbackUrl: string;
  outputDir: string;
}

export type AppPage = "poolgen" | "studio" | "history" | "mailpool" | "register" | "gptpool" | "settings";
