import { createBrowserRouter, RouterProvider } from 'react-router';
import { HomePage } from '@/features/home/HomePage';
import { MeetingPage } from '@/features/meeting/MeetingPage';
import { NotFoundPage } from '@/features/meeting/NotFoundPage';

const router = createBrowserRouter([
  { path: '/', element: <HomePage /> },
  { path: '/m/:code', element: <MeetingPage /> },
  { path: '*', element: <NotFoundPage /> },
]);

export function App() {
  return <RouterProvider router={router} />;
}
