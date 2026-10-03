import { api, type Provider, type Group, type Model } from './api'

export const app = $state({
  authenticated: false,
})

class ProvidersStore {
  providers = $state<Provider[]>([])
  groups = $state<Group[]>([])
  models = $state<Model[]>([])
  loaded = $state(false)
  error = $state('')

  async refresh() {
    try {
      const [providers, groups, models] = await Promise.all([
        api.providers(),
        api.groups(),
        api.models(),
      ])
      this.providers = providers ?? []
      this.groups = groups ?? []
      this.models = models ?? []
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
