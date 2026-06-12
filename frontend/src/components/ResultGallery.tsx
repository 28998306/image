import {useState} from "react";
import {OpenOutputDir, SaveRemoteFile} from "../../wailsjs/go/main/App";
import type {GalleryImage} from "../types";

interface ResultGalleryProps {
  images: GalleryImage[];
  selectedId: string;
  onSelect: (id: string) => void;
}

const modeLabels = {
  text: "文生图",
  image: "图生图",
  video: "视频生成",
};

export function ResultGallery({images, selectedId, onSelect}: ResultGalleryProps) {
  const selectedImage = images.find((image) => image.id === selectedId) || images[0];
  const [saveStatus, setSaveStatus] = useState("");

  const saveSelectedFile = async () => {
    if (!selectedImage) {
      return;
    }
    if (selectedImage.localPath) {
      setSaveStatus(`已保存：${selectedImage.localPath}`);
      return;
    }
    const path = await SaveRemoteFile(selectedImage.videoUrl || selectedImage.imageUrl || "", selectedImage.title);
    setSaveStatus(path ? `已保存：${path}` : "保存失败");
  };

  return (
    <section className="panel min-h-0 flex-1">
      <div className="panel-header flex items-center justify-between">
        <div>
          <h2 className="section-title">生成结果</h2>
          <p className="section-desc">先看大图，再从底部切换历史结果</p>
        </div>
        <div className="flex gap-2">
          <button className="btn-secondary" type="button" onClick={OpenOutputDir}>
            打开目录
          </button>
        </div>
      </div>

      {images.length === 0 ? (
        <div className="flex h-[calc(100%-65px)] items-center justify-center p-6">
          <div className="notice-box max-w-[360px] text-center">
            暂无生成结果。配置好 API Key 后，输入提示词并点击“开始生成”。
          </div>
        </div>
      ) : (
      <div className="flex h-[calc(100%-65px)] min-h-0 flex-col gap-4 p-4">
        <div className="grid min-h-0 flex-1 grid-cols-[1fr_220px] gap-4">
          <div className={`flex min-h-[260px] items-center justify-center border border-slate-200 bg-gradient-to-br ${selectedImage.palette}`}>
            {selectedImage.videoUrl ? (
              <video className="h-full max-h-[430px] w-full bg-black" controls poster={selectedImage.coverUrl || selectedImage.imageUrl}>
                <source src={selectedImage.videoUrl} type="video/mp4" />
              </video>
            ) : selectedImage.imageUrl ? (
              <img alt={selectedImage.title} className="h-full max-h-[430px] w-full object-contain" src={selectedImage.imageUrl} />
            ) : null}
          </div>

          <div className="flex flex-col border border-slate-200 bg-slate-50 p-4">
            <span className="tag w-fit min-h-0 px-2 py-1 text-[11px]">{modeLabels[selectedImage.mode]}</span>
            <h3 className="mt-4 text-base font-medium text-slate-900">{selectedImage.title}</h3>
            <div className="mt-2 text-xs leading-5 text-slate-600">{selectedImage.prompt}</div>

            <dl className="mt-4 space-y-2 text-xs">
              <div className="flex justify-between">
                <dt className="text-slate-500">尺寸</dt>
                <dd className="text-slate-800">{selectedImage.size}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-slate-500">时间</dt>
                <dd className="text-slate-800">{selectedImage.createdAt}</dd>
              </div>
            </dl>

            <div className="mt-auto grid gap-2 pt-4">
              <button className="btn-primary w-full" type="button" onClick={saveSelectedFile}>
                {selectedImage.videoUrl ? "保存视频" : "保存图片"}
              </button>
              <button className="btn-secondary w-full" type="button">
                复用参数
              </button>
              {saveStatus ? <div className="truncate text-[11px] text-slate-500">{saveStatus}</div> : null}
            </div>
          </div>
        </div>

        <div className="grid grid-cols-4 gap-3">
          {images.map((image) => (
            <button
              key={image.id}
              className={`overflow-hidden border bg-white text-left transition ${
                selectedId === image.id ? "border-brand-600" : "border-slate-200 hover:border-blue-300"
              }`}
              type="button"
              onClick={() => onSelect(image.id)}
            >
              <div className={`flex h-16 items-center justify-center bg-gradient-to-br ${image.palette}`}>
                {image.imageUrl ? <img alt={image.title} className="h-full w-full object-cover" src={image.imageUrl} /> : null}
                {image.videoUrl && !image.imageUrl ? <span className="text-xs text-slate-500">视频</span> : null}
              </div>
              <div className="p-2">
                <div className="truncate text-xs text-slate-900">{image.title}</div>
                <div className="mt-1 text-[11px] text-slate-500">{image.createdAt}</div>
              </div>
            </button>
          ))}
        </div>
      </div>
      )}
    </section>
  );
}
