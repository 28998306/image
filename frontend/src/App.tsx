import {useState} from "react";
import {ConfirmHost} from "./components/ConfirmDialog";
import {Sidebar} from "./components/Sidebar";
import {TopBar} from "./components/TopBar";
import {GptPoolPage} from "./pages/GptPoolPage";
import {HistoryPage} from "./pages/HistoryPage";
import {MailPoolPage} from "./pages/MailPoolPage";
import {PoolGenPage} from "./pages/PoolGenPage";
import {RegisterPoolPage} from "./pages/RegisterPoolPage";
import {SettingsPage} from "./pages/SettingsPage";
import {StudioPage} from "./pages/StudioPage";
import type {AppPage} from "./types";

function App() {
  const [activePage, setActivePage] = useState<AppPage>("poolgen");
  const [statsVersion, setStatsVersion] = useState(0);
  const refreshStats = () => setStatsVersion((version) => version + 1);

  // 号池生图页面始终挂载（仅隐藏），切换菜单不丢失任务列表与进行中的任务。
  const otherPage =
    activePage === "settings" ? (
      <SettingsPage />
    ) : activePage === "history" ? (
      <HistoryPage statsVersion={statsVersion} />
    ) : activePage === "mailpool" ? (
      <MailPoolPage />
    ) : activePage === "register" ? (
      <RegisterPoolPage />
    ) : activePage === "gptpool" ? (
      <GptPoolPage />
    ) : activePage === "studio" ? (
      <StudioPage onStatsUpdate={refreshStats} />
    ) : null;

  return (
    <div className="app-shell flex">
      <Sidebar activePage={activePage} statsVersion={statsVersion} onNavigate={setActivePage} />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar statsVersion={statsVersion} onOpenSettings={() => setActivePage("settings")} />
        <div className={activePage === "poolgen" ? "flex min-h-0 flex-1 flex-col" : "hidden"}>
          <PoolGenPage onStatsUpdate={refreshStats} />
        </div>
        {activePage !== "poolgen" ? otherPage : null}
      </div>
      <ConfirmHost />
    </div>
  );
}

export default App;
