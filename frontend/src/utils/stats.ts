import type {HistoryItem} from "../types";

export function isToday(isoString: string): boolean {
  if (!isoString) {
    return false;
  }

  const created = new Date(isoString);
  if (Number.isNaN(created.getTime())) {
    return false;
  }

  const now = new Date();
  return (
    created.getFullYear() === now.getFullYear() &&
    created.getMonth() === now.getMonth() &&
    created.getDate() === now.getDate()
  );
}

export function getTodayStats(history: HistoryItem[]) {
  const todayItems = history.filter((item) => isToday(item.createdAt));
  const images = todayItems.filter((item) => item.mode === "text" || item.mode === "image").length;
  const videos = todayItems.filter((item) => item.mode === "video").length;

  return {images, videos, total: todayItems.length};
}

export function formatTodayStats(images: number, videos: number) {
  if (images === 0 && videos === 0) {
    return "0 项";
  }

  const parts: string[] = [];
  if (images > 0) {
    parts.push(`${images} 张`);
  }
  if (videos > 0) {
    parts.push(`${videos} 条`);
  }
  return parts.join(" · ");
}
