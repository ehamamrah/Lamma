import { useEffect } from 'react';
import { Navigate, useParams } from 'react-router';
import { normalizeCode } from '@/shared/lib/code';
import { useMeetingStore } from '@/stores/meeting';
import { PreJoin } from '@/features/prejoin/PreJoin';
import { MeetingRoom } from '@/features/room/MeetingRoom';
import { WaitingRoom } from '@/features/waiting/WaitingRoom';
import { NotFoundPage } from './NotFoundPage';
import { loadSession } from './session';
import { StatusScreen } from './StatusScreen';

export function MeetingPage() {
  const { code: raw = '' } = useParams();
  const code = normalizeCode(raw);
  if (!code) return <NotFoundPage />;
  if (code !== raw) return <Navigate to={`/m/${code}`} replace />;
  return <MeetingFlow key={code} code={code} />;
}

function MeetingFlow({ code }: { code: string }) {
  const stage = useMeetingStore((s) => s.stage);
  useEffect(() => {
    void loadSession(code);
  }, [code]);

  switch (stage) {
    case 'loading':
      return <StatusScreen code={code} stage="loading" />;
    case 'not_found':
      return <NotFoundPage />;
    case 'prejoin':
    case 'connecting':
      return <PreJoin code={code} />;
    case 'waiting':
      return <WaitingRoom code={code} />;
    case 'in_room':
      return <MeetingRoom code={code} />;
    default:
      return <StatusScreen code={code} stage={stage} />;
  }
}
