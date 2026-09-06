import { mount } from 'svelte'
import '@fontsource/ibm-plex-sans/400.css'
import '@fontsource/ibm-plex-sans/500.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import './app.css'
import App from './App.svelte'

const app = mount(App, { target: document.getElementById('app')! })

export default app
