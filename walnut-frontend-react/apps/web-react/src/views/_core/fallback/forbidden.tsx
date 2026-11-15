import { Fallback } from './fallback';

export function Forbidden() {
  return <Fallback status="403" />;
}

export default Forbidden;
