let current = $state<'light' | 'dark'>(
  document.documentElement.dataset.theme === 'light' ? 'light' : 'dark',
)

export function theme() {
  return current
}

export function toggleTheme() {
  current = current === 'light' ? 'dark' : 'light'
  if (current === 'light') document.documentElement.dataset.theme = 'light'
  else delete document.documentElement.dataset.theme
  try {
    localStorage.setItem('pogu-theme', current)
  } catch {
    /* storage unavailable — theme just won't persist */
  }
}
