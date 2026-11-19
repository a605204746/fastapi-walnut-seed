import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { Button, Card, Space } from 'antd';
import { dictTypeInfo } from '@/api/system/dict/dict-type';
import type { DictType } from '@/api/system/dict/dict-type-model';
import { Page } from '@/components/page/Page';
import { DictDataModal, type DictDataModalHandle } from './data/dict-data-modal';
export default function DictionaryDataRoute() {
  const { dictId = '' } = useParams(); const navigate = useNavigate();
  const modal = useRef<DictDataModalHandle>(null); const [record, setRecord] = useState<DictType>();
  useEffect(() => { let cancelled = false; void dictTypeInfo(dictId).then(data => { if (!cancelled) { setRecord(data); modal.current?.open(data); } }); return () => { cancelled = true; }; }, [dictId]);
  return <Page><Card title={record?.dictName ?? '字典数据'}><Space><Button type="primary" disabled={!record} onClick={() => record && modal.current?.open(record)}>管理字典数据</Button><Button onClick={() => navigate('/system/dict')}>返回字典类型</Button></Space></Card><DictDataModal ref={modal} /></Page>;
}
