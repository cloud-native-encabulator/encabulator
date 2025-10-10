# Encabulation Control Console - Vue 3 Tech Stack

## Overview

The enhanced Encabulation Control Console will be built with Vue 3, providing a modern, interactive, and mobile-responsive interface for controlling and monitoring the Cloud Native Encabulator system.

## Frontend Stack

### Core Framework
- **Vue 3.4+** - Progressive JavaScript framework with Composition API
- **TypeScript 5.8+** - Type-safe development
- **Vite 6.0+** - Fast build tool and dev server

### Routing & State Management
- **Vue Router 4.3+** - Official routing library for Vue
- **Pinia 2.1+** - Intuitive state management (Vuex successor)

### UI & Styling
- **Tailwind CSS 3.4+** - Utility-first CSS framework
- **Headless UI Vue 1.7+** - Unstyled, accessible UI components
- **@vueuse/core 10.9+** - Collection of essential Vue composition utilities

### Data Visualization
- **D3.js 7.9+** - Data-driven visualizations
- **Force-directed graphs** - Interactive service topology
- **Particle animations** - Data flow visualization

### Real-time Communication
- **Socket.io-client 4.8+** - WebSocket client for real-time updates
- **Axios 1.7+** - HTTP client for REST API calls

### Testing
- **Vitest 2.0+** - Fast unit test framework
- **@vue/test-utils 2.4+** - Official Vue testing utilities

## Backend Stack (Existing - Enhanced)

### Core
- **Node.js 18+** - JavaScript runtime
- **Express** - Web framework
- **Socket.io** - WebSocket server
- **TypeScript** - Type-safe backend

### New Additions
- **jsonwebtoken** - JWT authentication
- **express-rate-limit** - Rate limiting middleware
- **http-proxy-middleware** - Grafana proxy

## Project Structure

```
services/control-plane/encabulation-control-console/
├── src/                          # Vue 3 frontend
│   ├── assets/                   # Static assets
│   ├── components/               # Vue components
│   │   ├── common/              # Shared components
│   │   ├── topology/            # System topology components
│   │   ├── actions/             # Action panel components
│   │   └── admin/               # Admin portal components
│   ├── composables/             # Vue composables
│   │   ├── useWebSocket.ts      # WebSocket connection
│   │   ├── useSystemData.ts     # System data management
│   │   └── useAuth.ts           # Authentication
│   ├── stores/                  # Pinia stores
│   │   ├── system.ts            # System state
│   │   ├── auth.ts              # Authentication state
│   │   └── actions.ts           # Action history
│   ├── router/                  # Vue Router configuration
│   │   └── index.ts             # Route definitions
│   ├── views/                   # Page components
│   │   ├── Console.vue          # Main console view
│   │   ├── Login.vue            # Login page
│   │   └── admin/               # Admin portal views
│   ├── types/                   # TypeScript types
│   ├── utils/                   # Utility functions
│   ├── App.vue                  # Root component
│   └── main.ts                  # Application entry
├── server/                      # Node.js backend
│   ├── src/
│   │   ├── middleware/          # Express middleware
│   │   ├── services/            # Business logic
│   │   ├── routes/              # API routes
│   │   └── index.ts             # Server entry
│   └── package.json
├── public/                      # Static files
├── index.html                   # HTML entry
├── vite.config.ts              # Vite configuration
├── tailwind.config.js          # Tailwind configuration
├── tsconfig.json               # TypeScript configuration
└── package.json                # Frontend dependencies
```

## Key Dependencies

### Frontend package.json
```json
{
  "dependencies": {
    "vue": "^3.4.0",
    "vue-router": "^4.3.0",
    "pinia": "^2.1.0",
    "socket.io-client": "^4.8.0",
    "d3": "^7.9.0",
    "axios": "^1.7.0",
    "@vueuse/core": "^10.9.0",
    "@headlessui/vue": "^1.7.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "vite": "^6.0.0",
    "typescript": "^5.8.0",
    "tailwindcss": "^3.4.0",
    "autoprefixer": "^10.4.0",
    "postcss": "^8.4.0",
    "vitest": "^2.0.0",
    "@vue/test-utils": "^2.4.0",
    "jsdom": "^25.0.0"
  }
}
```

### Backend package.json additions
```json
{
  "dependencies": {
    "jsonwebtoken": "^9.0.0",
    "express-rate-limit": "^7.0.0",
    "http-proxy-middleware": "^3.0.0"
  }
}
```

## Design System

### Color Palette (Dark Theme)
```css
/* Tailwind config */
colors: {
  background: '#0a0a0a',
  surface: '#1a1a1a',
  primary: '#00d9ff',      /* Cyan accent */
  secondary: '#ff00ff',    /* Magenta accent */
  success: '#00ff88',      /* Green */
  warning: '#ffaa00',      /* Orange */
  danger: '#ff0055',       /* Red */
  text: {
    primary: '#ffffff',
    secondary: '#a0a0a0',
    muted: '#666666'
  }
}
```

### Typography
- **Font Family**: Inter (system font fallback)
- **Headings**: Bold, uppercase for section titles
- **Body**: Regular weight, comfortable line height
- **Code**: Monospace for technical data

### Spacing & Layout
- **Mobile-first**: Base styles for mobile, scale up
- **Breakpoints**: sm (640px), md (768px), lg (1024px), xl (1280px)
- **Grid**: 12-column responsive grid
- **Spacing**: 4px base unit (Tailwind default)

## Component Patterns

### Composition API Example
```vue
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useSystemStore } from '@/stores/system'
import { useWebSocket } from '@/composables/useWebSocket'

const systemStore = useSystemStore()
const { connected, sendMessage } = useWebSocket()

const services = computed(() => systemStore.services)

function handleAction(action: string) {
  sendMessage({ type: action, payload: {} })
}

onMounted(() => {
  // Component initialization
})
</script>

<template>
  <div class="p-4 bg-surface rounded-lg">
    <h2 class="text-xl font-bold text-primary">System Status</h2>
    <div v-if="connected" class="text-success">Connected</div>
    <div v-else class="text-danger">Disconnected</div>
  </div>
</template>
```

### Pinia Store Example
```typescript
// stores/system.ts
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useSystemStore = defineStore('system', () => {
  const services = ref<Record<string, Service>>({})
  const metrics = ref<Record<string, number>>({})
  
  const healthyServices = computed(() => 
    Object.values(services.value).filter(s => s.status === 'healthy')
  )
  
  function updateServices(data: Record<string, Service>) {
    services.value = data
  }
  
  return { 
    services, 
    metrics, 
    healthyServices,
    updateServices 
  }
})
```

## Mobile Responsiveness

### Touch Interactions
- **Tap targets**: Minimum 44x44px
- **Swipe gestures**: Navigate between views
- **Pinch to zoom**: On topology visualization
- **Pull to refresh**: Update system status

### Responsive Breakpoints
- **Mobile (< 640px)**: Single column, stacked panels
- **Tablet (640-1024px)**: Two columns, side-by-side
- **Desktop (> 1024px)**: Full layout with sidebar

### Performance Optimizations
- **Code splitting**: Lazy load admin portal
- **Image optimization**: WebP with fallbacks
- **Bundle size**: < 500KB initial load
- **Lighthouse score**: > 90 on mobile

## Accessibility

### WCAG 2.1 AA Compliance
- **Keyboard navigation**: Full keyboard support
- **Screen readers**: ARIA labels and roles
- **Color contrast**: 4.5:1 minimum ratio
- **Focus indicators**: Visible focus states

### Headless UI Benefits
- Built-in accessibility
- Keyboard navigation
- Screen reader support
- Focus management

## Development Workflow

### Local Development
```bash
# Frontend
npm install
npm run dev          # Start Vite dev server (port 5173)

# Backend
cd server
npm install
npm run dev          # Start Express server (port 8080)
```

### Building for Production
```bash
npm run build        # Build Vue app to dist/
npm run preview      # Preview production build
```

### Testing
```bash
npm run test         # Run Vitest tests
npm run test:ui      # Run tests with UI
```

## Docker Build

### Multi-stage Dockerfile
```dockerfile
# Stage 1: Build Vue frontend
FROM node:18-alpine AS frontend-build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Stage 2: Build backend
FROM node:18-alpine AS backend-build
WORKDIR /app/server
COPY server/package*.json ./
RUN npm ci
COPY server/ .
RUN npm run build

# Stage 3: Production
FROM node:18-alpine
WORKDIR /app
COPY --from=backend-build /app/server/dist ./server
COPY --from=backend-build /app/server/node_modules ./node_modules
COPY --from=frontend-build /app/dist ./public
EXPOSE 8080
CMD ["node", "server/index.js"]
```

## Next Steps

1. **Task 6.1**: Set up Vue 3 project with all dependencies
2. **Task 6.2**: Migrate core components to Vue
3. **Task 6.3**: Implement encabulator-themed actions
4. **Task 6.4**: Add user management API
5. **Task 6.5**: Build admin portal
6. **Task 6.6**: Enhance backend with new features

## Resources

- [Vue 3 Documentation](https://vuejs.org/)
- [Pinia Documentation](https://pinia.vuejs.org/)
- [Tailwind CSS](https://tailwindcss.com/)
- [Headless UI Vue](https://headlessui.com/vue)
- [VueUse](https://vueuse.org/)
- [D3.js](https://d3js.org/)
