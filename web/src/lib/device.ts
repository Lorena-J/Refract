// The browser and system a user agent names, to tell sessions apart. Names
// are proper nouns, so they aren't translated; null when it can't tell.
export type Device = { browser: string | null; os: string | null }

const browsers: [RegExp, string][] = [
  [/\bEdgA?\/|\bEdgiOS\//, 'Edge'],
  [/\bOPR\/|\bOpera\b/, 'Opera'],
  [/\bSamsungBrowser\//, 'Samsung Internet'],
  [/\bFirefox\/|\bFxiOS\//, 'Firefox'],
  [/\bChrome\/|\bCriOS\/|\bChromium\//, 'Chrome'],
  [/\bVersion\/[\d.]+.*\bSafari\//, 'Safari'],
]

const systems: [RegExp, string][] = [
  [/\biPad\b/, 'iPadOS'],
  [/\biPhone\b|\biPod\b/, 'iOS'],
  [/\bAndroid\b/, 'Android'],
  [/\bCrOS\b/, 'ChromeOS'],
  [/\bWindows\b/, 'Windows'],
  [/\bMac OS X\b|\bMacintosh\b/, 'macOS'],
  [/\bLinux\b/, 'Linux'],
]

export function device(ua: string): Device {
  const find = (list: [RegExp, string][]) => list.find(([re]) => re.test(ua))?.[1] ?? null
  return { browser: find(browsers), os: find(systems) }
}
