import type {CSSProperties} from "react";
import {useEffect, useState} from "react";
import {LoadHistory} from "../../wailsjs/go/main/App";
import {BrowserOpenURL, Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise} from "../../wailsjs/runtime";
import type {HistoryItem} from "../types";
import {formatTodayStats, getTodayStats} from "../utils/stats";

const dragStyle = {"--wails-draggable": "drag"} as CSSProperties;
const noDragStyle = {"--wails-draggable": "no-drag"} as CSSProperties;

function MinimizeIcon() {
  return (
    <svg aria-hidden="true" fill="none" height="14" viewBox="0 0 14 14" width="14">
      <path d="M3 8.5h8" stroke="currentColor" strokeLinecap="round" strokeWidth="1.4" />
    </svg>
  );
}

function MaximizeIcon({isMaximised}: {isMaximised: boolean}) {
  if (isMaximised) {
    return (
      <svg aria-hidden="true" fill="none" height="14" viewBox="0 0 14 14" width="14">
        <path d="M5 3.5h5.5v5.5" stroke="currentColor" strokeWidth="1.2" />
        <path d="M3.5 5h5.5v5.5h-5.5z" stroke="currentColor" strokeWidth="1.2" />
      </svg>
    );
  }

  return (
    <svg aria-hidden="true" fill="none" height="14" viewBox="0 0 14 14" width="14">
      <path d="M3.5 3.5h7v7h-7z" stroke="currentColor" strokeWidth="1.2" />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg aria-hidden="true" fill="none" height="14" viewBox="0 0 14 14" width="14">
      <path d="M4 4l6 6M10 4l-6 6" stroke="currentColor" strokeLinecap="round" strokeWidth="1.4" />
    </svg>
  );
}

interface TopBarProps {
  onOpenSettings: () => void;
  statsVersion: number;
}

export function TopBar({onOpenSettings, statsVersion}: TopBarProps) {
  const [isMaximised, setIsMaximised] = useState(false);
  const [todayStatsText, setTodayStatsText] = useState("0 项");

  useEffect(() => {
    WindowIsMaximised().then(setIsMaximised).catch(() => setIsMaximised(false));
  }, []);

  useEffect(() => {
    LoadHistory()
      .then((items) => {
        const stats = getTodayStats(items as HistoryItem[]);
        setTodayStatsText(formatTodayStats(stats.images, stats.videos));
      })
      .catch(() => setTodayStatsText("0 项"));
  }, [statsVersion]);

  const toggleMaximise = async () => {
    WindowToggleMaximise();
    window.setTimeout(() => {
      WindowIsMaximised().then(setIsMaximised).catch(() => setIsMaximised(false));
    }, 120);
  };

  return (
    <header className="flex h-12 select-none items-center justify-between border-b border-slate-200 bg-white" style={dragStyle}>
      <div className="flex h-full items-center gap-4 px-5" onDoubleClick={toggleMaximise}>
        <div className="border-r border-slate-200 pr-4">
          <div className="text-sm font-medium text-slate-900">Web2Img AI Studio</div>
          <div className="text-[11px] text-slate-500">企业品牌图像创作</div>
        </div>
        <div className="flex items-center gap-2 text-xs text-slate-500" style={noDragStyle}>
          <span className="hidden lg:inline">蓝色企业风 · 本地桌面工作台</span>
          <span className="hidden text-slate-300 lg:inline">·</span>
          <button
            className="font-medium text-brand-700 hover:underline"
            type="button"
            onClick={() => BrowserOpenURL("https://github.com/28998306/image")}
          >
            开源地址 github.com/28998306/image
          </button>
          <span className="text-slate-300">·</span>
          <span>QQ 交流群 19302577</span>
        </div>
      </div>

      <div className="flex h-full items-center" style={noDragStyle}>
        <div className="status-chip mr-3">
          今日生成 <span className="text-brand-700">{todayStatsText}</span>
        </div>
        <div className="status-chip status-chip-success mr-3">
          API 正常
        </div>
        <button className="btn-secondary mr-3" type="button" onClick={onOpenSettings}>
          全局设置
        </button>
        <div className="mr-2 flex items-center gap-1 border-l border-slate-200 pl-2">
          <button
            aria-label="最小化"
            className="window-control"
            type="button"
            onClick={WindowMinimise}
          >
            <MinimizeIcon />
          </button>
          <button
            aria-label={isMaximised ? "还原" : "最大化"}
            className="window-control"
            type="button"
            onClick={toggleMaximise}
          >
            <MaximizeIcon isMaximised={isMaximised} />
          </button>
          <button
            aria-label="关闭"
            className="window-control window-control-close"
            type="button"
            onClick={Quit}
          >
            <CloseIcon />
          </button>
        </div>
      </div>
    </header>
  );
}
