// Shared formatters use the initialized instance without importing desktop
// resource loading. Each application entry point owns initialization.
import i18n from "i18next";
i18n.on("languageChanged", language => {
  if (typeof document !== "undefined") document.documentElement.lang = language;
});

export default i18n;
