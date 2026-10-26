// t() over the shared catalog in ../locales (also used by Go). en.json is the
// reference set of keys: `it` is typed as `typeof en`, so a key missing in
// it.json fails svelte-check.
import en from "../../../locales/en.json";
import itJSON from "../../../locales/it.json";
import { Langs, type Lang } from "./api";

export type Key = keyof typeof en;
const it: typeof en = itJSON;

let current = $state<Lang>(Langs.IT);

export function setLang(l: Lang): void {
  current = l;
  document.documentElement.lang = l;
}

export function lang(): Lang {
  return current;
}

/** The text for key in the current language, with `{name}` placeholders filled from params. */
export function t(key: Key, params: Record<string, string> = {}): string {
  const text = (current === Langs.EN ? en : it)[key];
  return text.replace(/\{(\w+)\}/g, (m, name: string) => params[name] ?? m);
}

/** The text for an error code (`error.<code>`), falling back to `error.internal`. */
export function tError(code: string, params: Record<string, string> = {}): string {
  const key = `error.${code}`;
  return key in en ? t(key as Key, params) : t("error.internal");
}
