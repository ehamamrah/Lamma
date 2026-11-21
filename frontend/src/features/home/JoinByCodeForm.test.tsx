import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { JoinByCodeForm } from './JoinByCodeForm';

function setup() {
  const router = createMemoryRouter(
    [
      { path: '/', element: <JoinByCodeForm /> },
      { path: '/m/:code', element: <p>meeting page</p> },
    ],
    { initialEntries: ['/'] },
  );
  render(<RouterProvider router={router} />);
  return router;
}

describe('JoinByCodeForm', () => {
  it('navigates to the canonical meeting URL', async () => {
    const router = setup();
    await userEvent.type(screen.getByLabelText(/meeting code/i), 'ABCD EFGH JKMN');
    await userEvent.click(screen.getByRole('button', { name: /join/i }));
    expect(router.state.location.pathname).toBe('/m/abcd-efgh-jkmn');
  });

  it('rejects input that is not a code', async () => {
    setup();
    await userEvent.type(screen.getByLabelText(/meeting code/i), 'hello');
    await userEvent.click(screen.getByRole('button', { name: /join/i }));
    expect(screen.getByRole('alert')).toHaveTextContent(/does not look like/i);
  });
});
