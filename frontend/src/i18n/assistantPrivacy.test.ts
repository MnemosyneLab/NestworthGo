import { expect, it } from "vitest";
import i18n from "./index";

// Pin the privacy claims independently of the component's translation calls.
// The settings and assistant surfaces share the storage and activation wording.
it.each([
  { language: "en", settings: "desktop Settings", backup: ["without field encryption", "main household database", "Full backups include this key", "deleting it does not remove copies in older backups"], gates: "Saving a key does not activate model calls or financial writes.", memory: ["Only local page explanations", "Backend AI sessions have separate persistent storage."] },
  { language: "zh-CN", settings: "桌面端设置", backup: ["未经字段加密", "家庭主数据库", "完整备份包含此密钥", "删除密钥不会清除旧备份中的副本"], gates: "保存密钥不会开启模型调用或财务写入。", memory: ["仅本地页面解释", "后端 AI 会话使用独立的持久存储。"] },
  { language: "zh-TW", settings: "桌面端設定", backup: ["未經欄位加密", "家庭主資料庫", "完整備份包含此金鑰", "刪除金鑰不會清除舊備份中的副本"], gates: "儲存金鑰不會開啟模型呼叫或財務寫入。", memory: ["僅本機頁面解釋", "後端 AI 對話使用獨立的持久儲存。"] },
])("keeps accurate credential and local explanation claims in $language", ({ language, settings, backup, gates, memory }) => {
  const resource = (key: string) => i18n.getResource(language, "translation", key) as string;
  expect(resource("assistant.credentialSettings")).toContain(settings);
  for (const claim of backup) expect(resource("aiCredentials.backup")).toContain(claim);
  expect(resource("aiCredentials.help")).toContain(gates);
  for (const claim of memory) expect(resource("assistant.memoryOnly")).toContain(claim);
  expect(resource("assistant.secureStoreUnavailable")).toBeUndefined();
});
