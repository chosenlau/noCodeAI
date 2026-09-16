import { Routes, Route, Navigate } from 'react-router-dom';
import { Toaster } from '@/components/ui/toaster';
import LoginPage from '@/pages/LoginPage';
import RegisterPage from '@/pages/RegisterPage';
import AppsPage from '@/pages/AppsPage';
import ChatPage from '@/pages/ChatPage';
import ChatPageDebug from '@/pages/ChatPageDebug';
import ProtectedRoute from '@/components/ProtectedRoute';

function App() {
  return (
    <>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route element={<ProtectedRoute />}>
          <Route path="/apps" element={<AppsPage />} />
          <Route path="/chat/:appId" element={<ChatPage />} />
          <Route path="/debug/:appId" element={<ChatPageDebug />} />
        </Route>
        <Route path="/" element={<Navigate to="/apps" replace />} />
      </Routes>
      <Toaster />
    </>
  );
}

export default App;
