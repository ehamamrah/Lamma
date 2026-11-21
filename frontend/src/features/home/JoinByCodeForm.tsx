import { type FormEvent, useState } from 'react';
import { useNavigate } from 'react-router';
import { normalizeCode } from '@/shared/lib/code';
import { Alert } from '@/shared/ui/alert';
import { Button } from '@/shared/ui/button';
import { Card, CardDescription, CardTitle } from '@/shared/ui/card';
import { Field } from '@/shared/ui/field';
import { Input } from '@/shared/ui/input';

export function JoinByCodeForm() {
  const navigate = useNavigate();
  const [value, setValue] = useState('');
  const [error, setError] = useState<string | null>(null);

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const code = normalizeCode(value);
    if (!code) {
      setError('That does not look like a meeting code or link.');
      return;
    }
    navigate(`/m/${code}`);
  }

  return (
    <Card>
      <form className="flex flex-col gap-4" onSubmit={onSubmit}>
        <div>
          <CardTitle>Join a meeting</CardTitle>
          <CardDescription>Paste the link or code you were given.</CardDescription>
        </div>
        <Field id="code" label="Meeting code or link">
          <Input
            id="code"
            required
            placeholder="abcd-efgh-jkmn"
            value={value}
            onChange={(e) => {
              setValue(e.target.value);
              setError(null);
            }}
          />
        </Field>
        {error && <Alert>{error}</Alert>}
        <Button type="submit" variant="secondary" size="lg">
          Join
        </Button>
      </form>
    </Card>
  );
}
