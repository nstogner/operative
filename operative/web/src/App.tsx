import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { OperativeList } from './pages/OperativeList';
import { OperativeDetail } from './pages/OperativeDetail';
import './index.css';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Navigate to="/operatives" replace />} />
        <Route path="/operatives" element={<OperativeList />} />
        <Route path="/operatives/:id" element={<OperativeDetail />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
