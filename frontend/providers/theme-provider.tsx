"use client"

import * as React from "react"

export type Theme = "light" | "dark" | "system"

interface ThemeContextType {
  theme: Theme
  setTheme: (theme: Theme) => void
  resolvedTheme: "light" | "dark"
}

const ThemeContext = React.createContext<ThemeContextType | undefined>(undefined)

const THEME_STORAGE_KEY = "opsflow_theme"

export function ThemeProvider({
  children,
  defaultTheme = "system",
}: {
  children: React.ReactNode
  attribute?: string
  defaultTheme?: Theme
  enableSystem?: boolean
  disableTransitionOnChange?: boolean
}) {
  const [theme, setThemeState] = React.useState<Theme>(defaultTheme)
  const [resolvedTheme, setResolvedTheme] = React.useState<"light" | "dark">("light")

  const applyTheme = React.useCallback((targetTheme: Theme) => {
    if (typeof window === "undefined") return

    let resolved: "light" | "dark" = "light"
    if (targetTheme === "system") {
      resolved = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
    } else {
      resolved = targetTheme
    }

    setResolvedTheme(resolved)
    const root = document.documentElement
    if (resolved === "dark") {
      root.classList.add("dark")
      root.style.colorScheme = "dark"
    } else {
      root.classList.remove("dark")
      root.style.colorScheme = "light"
    }
  }, [])

  React.useEffect(() => {
    let initialTheme = defaultTheme
    try {
      const stored = localStorage.getItem(THEME_STORAGE_KEY) as Theme | null
      if (stored && (stored === "light" || stored === "dark" || stored === "system")) {
        initialTheme = stored
      }
    } catch {
      // localStorage restricted
    }
    setThemeState(initialTheme)
    applyTheme(initialTheme)

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)")
    const handleMediaChange = () => {
      try {
        const current = localStorage.getItem(THEME_STORAGE_KEY) as Theme | null
        if (!current || current === "system") {
          applyTheme("system")
        }
      } catch {
        applyTheme("system")
      }
    }

    mediaQuery.addEventListener("change", handleMediaChange)
    return () => mediaQuery.removeEventListener("change", handleMediaChange)
  }, [defaultTheme, applyTheme])

  const setTheme = React.useCallback(
    (newTheme: Theme) => {
      setThemeState(newTheme)
      try {
        localStorage.setItem(THEME_STORAGE_KEY, newTheme)
      } catch {
        // localStorage restricted
      }
      applyTheme(newTheme)
    },
    [applyTheme]
  )

  const value = React.useMemo(
    () => ({
      theme,
      setTheme,
      resolvedTheme,
    }),
    [theme, setTheme, resolvedTheme]
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextType {
  const context = React.useContext(ThemeContext)
  if (!context) {
    throw new Error("useTheme must be used within a ThemeProvider")
  }
  return context
}
