// =============================================================================
// Argus AI — i18n message catalogs (dependency-free)
// =============================================================================
// Flat, dotted-key catalogs for KZ / RU / EN. Russian is the source/fallback.
// Add keys here and reference them via useLocale().t('nav.overview').
// =============================================================================

export type Locale = 'kk' | 'ru' | 'en'

/** Locales offered in the UI switcher, in display order. */
export const LOCALES: { code: Locale, label: string, name: string }[] = [
  { code: 'kk', label: 'KZ', name: 'Қазақша' },
  { code: 'ru', label: 'RU', name: 'Русский' },
  { code: 'en', label: 'EN', name: 'English' }
]

export const DEFAULT_LOCALE: Locale = 'ru'

type Catalog = Record<string, string>

export const messages: Record<Locale, Catalog> = {
  ru: {
    'nav.dashboard': 'Дашборд',
    'nav.overview': 'Обзор',
    'nav.regions': 'Регионы',
    'nav.violations': 'Нарушения',
    'nav.infrastructure': 'Инфраструктура',
    'nav.executive': 'Executive',
    'nav.exams': 'Экзамены',
    'nav.tests': 'Тесты',
    'nav.complexTests': 'Комплексные тесты',
    'nav.monitoring': 'Мониторинг',
    'nav.archive': 'Архив сессий',
    'nav.forensic': 'Форензик',
    'nav.appeals': 'Апелляции',
    'nav.exports': 'Экспорт',
    'nav.analytics': 'Аналитика',
    'nav.integrations': 'Интеграции',
    'nav.apiIntegrations': 'API и Интеграции',
    'nav.privacy': 'Приватность',
    'nav.organizations': 'Организации',
    'common.administration': 'Администрирование',
    'common.lightTheme': 'Светлая тема',
    'common.darkTheme': 'Тёмная тема',
    'common.logout': 'Выйти',
    'common.collapse': 'Свернуть',
    'common.expand': 'Развернуть',
    'common.language': 'Язык'
  },
  kk: {
    'nav.dashboard': 'Дашборд',
    'nav.overview': 'Шолу',
    'nav.regions': 'Аймақтар',
    'nav.violations': 'Бұзушылықтар',
    'nav.infrastructure': 'Инфрақұрылым',
    'nav.executive': 'Басшылық',
    'nav.exams': 'Емтихандар',
    'nav.tests': 'Тесттер',
    'nav.complexTests': 'Кешенді тесттер',
    'nav.monitoring': 'Мониторинг',
    'nav.archive': 'Сессиялар мұрағаты',
    'nav.forensic': 'Форензика',
    'nav.appeals': 'Апелляциялар',
    'nav.exports': 'Экспорт',
    'nav.analytics': 'Аналитика',
    'nav.integrations': 'Интеграциялар',
    'nav.apiIntegrations': 'API және интеграциялар',
    'nav.privacy': 'Құпиялылық',
    'nav.organizations': 'Ұйымдар',
    'common.administration': 'Әкімшілендіру',
    'common.lightTheme': 'Ашық тема',
    'common.darkTheme': 'Қараңғы тема',
    'common.logout': 'Шығу',
    'common.collapse': 'Жию',
    'common.expand': 'Жаю',
    'common.language': 'Тіл'
  },
  en: {
    'nav.dashboard': 'Dashboard',
    'nav.overview': 'Overview',
    'nav.regions': 'Regions',
    'nav.violations': 'Violations',
    'nav.infrastructure': 'Infrastructure',
    'nav.executive': 'Executive',
    'nav.exams': 'Exams',
    'nav.tests': 'Tests',
    'nav.complexTests': 'Complex Tests',
    'nav.monitoring': 'Monitoring',
    'nav.archive': 'Session Archive',
    'nav.forensic': 'Forensics',
    'nav.appeals': 'Appeals',
    'nav.exports': 'Export',
    'nav.analytics': 'Analytics',
    'nav.integrations': 'Integrations',
    'nav.apiIntegrations': 'API & Integrations',
    'nav.privacy': 'Privacy',
    'nav.organizations': 'Organizations',
    'common.administration': 'Administration',
    'common.lightTheme': 'Light theme',
    'common.darkTheme': 'Dark theme',
    'common.logout': 'Log out',
    'common.collapse': 'Collapse',
    'common.expand': 'Expand',
    'common.language': 'Language'
  }
}
