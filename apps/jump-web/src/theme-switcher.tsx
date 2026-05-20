import { THEME_CATALOG, shouldShowThemeSwitcher } from './appearance'
import { appearance, setThemeId } from './store'

export function ThemeMenuOptions({ onSelect }: { onSelect?: () => void }) {
  if (!shouldShowThemeSwitcher()) return null

  const activeThemeId = appearance.value.themeId

  return (
    <div class="theme-menu-options" role="radiogroup" aria-label="Web UI theme">
      {THEME_CATALOG.map(theme => {
        const active = theme.id === activeThemeId
        const label = `${theme.label} theme${active ? ' selected' : ''}`
        return (
          <button
            key={theme.id}
            type="button"
            class={`theme-menu-option${active ? ' active' : ''}`}
            role="radio"
            aria-checked={active}
            aria-label={label}
            title={label}
            onClick={() => {
              void setThemeId(theme.id)
              onSelect?.()
            }}
          >
            <span class={`theme-menu-swatch ${theme.id}`} aria-hidden="true" />
            {active && <span class="theme-menu-active-dot" aria-hidden="true" />}
          </button>
        )
      })}
    </div>
  )
}
