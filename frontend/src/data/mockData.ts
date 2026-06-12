import type {GalleryImage, QueueItem} from "../types";

export const samplers = ["gpt2api 默认", "文生图", "图生图", "图片编辑"];

export const mockGallery: GalleryImage[] = [
  {
    id: "img-1001",
    title: "企业宣传主视觉",
    mode: "text",
    prompt: "blue enterprise technology poster, clean lighting, modern office",
    createdAt: "16:28",
    size: "1024 x 1024",
    palette: "from-blue-700 via-sky-500 to-slate-200",
  },
  {
    id: "img-1002",
    title: "产品概念图",
    mode: "image",
    prompt: "industrial product render, precise edges, neutral background",
    createdAt: "16:21",
    size: "1344 x 768",
    palette: "from-slate-800 via-blue-600 to-cyan-200",
  },
  {
    id: "img-1003",
    title: "参考图重绘版本",
    mode: "image",
    prompt: "restyle with corporate blue data center",
    createdAt: "16:09",
    size: "768 x 1344",
    palette: "from-blue-900 via-indigo-500 to-blue-100",
  },
  {
    id: "img-1004",
    title: "电商横幅",
    mode: "text",
    prompt: "clean commercial banner, blue business style, sharp detail",
    createdAt: "15:56",
    size: "1536 x 864",
    palette: "from-sky-700 via-blue-500 to-slate-100",
  },
];

export const initialQueue: QueueItem[] = [
  {
    id: "job-2309",
    title: "品牌宣传图 04",
    status: "running",
    progress: 68,
    mode: "text",
  },
  {
    id: "job-2310",
    title: "产品背景替换",
    status: "waiting",
    progress: 0,
    mode: "image",
  },
  {
    id: "job-2307",
    title: "车间海报增强",
    status: "done",
    progress: 100,
    mode: "image",
  },
];
