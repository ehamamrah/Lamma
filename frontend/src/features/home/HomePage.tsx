import { Video } from 'lucide-react';
import { CreateMeetingForm } from './CreateMeetingForm';
import { JoinByCodeForm } from './JoinByCodeForm';

export function HomePage() {
  return (
    <main className="mx-auto flex min-h-full max-w-5xl flex-col justify-center gap-10 px-6 py-12">
      <header className="flex flex-col gap-3">
        <div className="flex items-center gap-2 text-primary">
          <Video aria-hidden />
          <span className="text-xl font-semibold tracking-tight">Lamma</span>
        </div>
        <h1 className="text-4xl font-semibold tracking-tight sm:text-5xl">
          Video meetings in one click.
        </h1>
        <p className="max-w-xl text-muted-foreground">
          No accounts, no downloads. Start a meeting, share the link, and talk.
        </p>
      </header>
      <div className="grid gap-6 md:grid-cols-2">
        <CreateMeetingForm />
        <JoinByCodeForm />
      </div>
    </main>
  );
}
