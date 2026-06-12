import type {GenerationMode} from "../types";

interface ImageInputPanelProps {
  mode: GenerationMode;
  referenceImages: string[];
  onReferenceImagesChange: (value: string[]) => void;
}

export function ImageInputPanel({mode, referenceImages, onReferenceImagesChange}: ImageInputPanelProps) {
  const handleFilesChange = (files?: FileList | null) => {
    if (!files || files.length === 0) {
      return;
    }

    Promise.all(
      Array.from(files).map(
        (file) =>
          new Promise<string>((resolve) => {
            const reader = new FileReader();
            reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
            reader.readAsDataURL(file);
          }),
      ),
    ).then((results) => {
      onReferenceImagesChange([...referenceImages, ...results.filter(Boolean)]);
    });
  };

  const removeImage = (index: number) => {
    onReferenceImagesChange(referenceImages.filter((_, currentIndex) => currentIndex !== index));
  };

  if (mode === "text") {
    return null;
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <h2 className="section-title">参考图输入</h2>
        <p className="section-desc">{mode === "video" ? "可选，上传后按图生视频调用" : "支持多张参考图，会通过 images 数组提交"}</p>
      </div>

      <div className="p-3">
        <label className="flex min-h-[120px] cursor-pointer items-center justify-center overflow-hidden border border-dashed border-blue-300 bg-blue-50 text-center">
          <input
            accept="image/png,image/jpeg,image/webp"
            className="hidden"
            multiple
            type="file"
            onChange={(event) => handleFilesChange(event.target.files)}
          />
          <div>
            <div className="text-sm font-medium text-brand-700">点击选择参考图</div>
            <div className="mt-1 text-xs text-slate-500">可一次选择多张 PNG / JPG / WEBP</div>
            <span className="btn-secondary mt-3 inline-flex items-center">选择文件</span>
          </div>
        </label>

        {referenceImages.length > 0 ? (
          <>
            <div className="mt-3 grid grid-cols-3 gap-2">
              {referenceImages.map((image, index) => (
                <div key={`${image.slice(0, 32)}-${index}`} className="relative border border-slate-200 bg-white">
                  <img alt={`参考图 ${index + 1}`} className="h-16 w-full object-cover" src={image} />
                  <button
                    className="absolute right-1 top-1 border border-slate-200 bg-white px-1 text-[10px] text-slate-600"
                    type="button"
                    onClick={() => removeImage(index)}
                  >
                    删除
                  </button>
                </div>
              ))}
            </div>

            <div className="mt-3 flex gap-2">
            <label className="btn-secondary inline-flex cursor-pointer items-center">
              <input
                accept="image/png,image/jpeg,image/webp"
                className="hidden"
                multiple
                type="file"
                onChange={(event) => handleFilesChange(event.target.files)}
              />
              添加图片
            </label>
            <button className="btn-secondary" type="button" onClick={() => onReferenceImagesChange([])}>
              清空
            </button>
            </div>
          </>
        ) : null}
      </div>
    </section>
  );
}
