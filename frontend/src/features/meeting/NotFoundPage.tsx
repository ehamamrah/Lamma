import { Link } from 'react-router';
import { Button } from '@/shared/ui/button';
import { Card, CardDescription, CardTitle } from '@/shared/ui/card';

export function NotFoundPage() {
  return (
    <main className="flex h-full items-center justify-center p-6">
      <Card className="flex max-w-md flex-col gap-4 text-center">
        <CardTitle>Meeting not found</CardTitle>
        <CardDescription>Check the link. The meeting may have ended or expired.</CardDescription>
        <Button asChild>
          <Link to="/">Start a new meeting</Link>
        </Button>
      </Card>
    </main>
  );
}
