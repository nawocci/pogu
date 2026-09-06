import { api, type Provider } from './api'

export const app = $state({
  authenticated: false,
})

class ProvidersStore {
  providers = $state<Provider[]>([])
  loaded = $state(false)
  error = $state('')

  async refresh() {
    try {
      const providers = await api.providers()
      this.providers = providers ?? []
      this.loaded = true
      this.error = ''
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e)
    }
  }

  async mutate<T>(fn: () => Promise<T>): Promise<T> {
    const result = await fn()
    await this.refresh()
    return result
  }
}

export const store = new ProvidersStore()
