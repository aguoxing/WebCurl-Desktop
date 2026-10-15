import { defineStore } from 'pinia'

export const useUserStore = defineStore('user', {
  state: () => ({ name: 'Alice', token: '' }),
  getters: { isLogin: (s) => !!s.token },
  actions: {
    login(name: string) {
      this.name = name
      this.token = 'token-' + Date.now()
    },
    logout() {
      this.token = ''
    }
  }
})
