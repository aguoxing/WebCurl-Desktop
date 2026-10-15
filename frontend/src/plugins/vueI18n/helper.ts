export const setHtmlPageLang = (locale: any) => {
  document.querySelector('html')?.setAttribute('lang', locale)
}
