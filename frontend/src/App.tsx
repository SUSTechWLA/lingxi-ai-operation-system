import React from 'react'
import Sidebar from './components/Sidebar'
import PublishPage from './pages/PublishPage'

function App() {
  return (
    <div className="flex h-screen">
      <Sidebar />
      <PublishPage />
    </div>
  )
}

export default App
