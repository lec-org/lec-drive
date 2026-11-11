import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import Backend from "i18next-http-backend";
import ChainedBackend from "i18next-chained-backend";
import LocalStorageBackend from "i18next-localstorage-backend";

// eslint-disable-next-line @typescript-eslint/ban-ts-ignore
// @ts-ignore
i18n
  .use(ChainedBackend)
  .use(initReactI18next)
  .init({
    // LecDrive 固定中文，浏览器偏好和旧账号语言均不能切换到其他语言。
    lng: "zh-CN",
    supportedLngs: ["zh-CN"],
    fallbackLng: "zh-CN",
    debug: false,
    ns: ["common", "application", "dashboard"],
    load: "currentOnly",
    defaultNS: "application",
    interpolation: {
      escapeValue: false,
    },
    backend: {
      backends: process.env.NODE_ENV === "development" ? [Backend] : [LocalStorageBackend, Backend],
      backendOptions: [
        {
          expirationTime: 7 * 24 * 60 * 60 * 1000, // 7 days
          loadPath: "/locales/{{lng}}/{{ns}}.json",
        },
      ],
    },
  });

i18n.on("languageChanged", (lng) => {
  document.documentElement.setAttribute("lang", lng);
});

export const languages = [
  {
    code: "zh-CN",
    displayName: "简体中文",
  },
];

export default i18n;
