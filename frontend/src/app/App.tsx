import { createBrowserRouter, RouterProvider } from 'react-router';
import { HomePage } from '@/features/home/HomePage';
import { NotFoundPage } from '@/features/meeting/NotFoundPage';

// The meeting route pulls in the LiveKit SDK; load it only when needed so the
// landing page stays small.
const router = createBrowserRouter([
  { path: '/', element: <HomePage /> },
  {
    path: '/m/:code',
    lazy: async () => {
      const { MeetingPage } = await import('@/features/meeting/MeetingPage');
      return { Component: MeetingPage };
    },
  },
  { path: '*', element: <NotFoundPage /> },
]);

export function App() {
  return <RouterProvider router={router} />;
}
