import { useEffect, useState } from 'react';
import { Button, Card, Descriptions, Space, Spin, Table } from 'antd';
import { useNavigate, useParams } from 'react-router';
import type { Role } from '@/api/system/role/model';
import { getUserAuthRole, userAuthRoleUpdate } from '@/api/system/user';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
export default function UserAuthRole() {
  const { userId = '' } = useParams();
  const [data, setData] = useState<{ user: any; roles: Role[] }>();
  const [selected, setSelected] = useState<React.Key[]>([]);
  const [saving, setSaving] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    let cancelled = false;
    void getUserAuthRole(userId).then((result: any) => { if (!cancelled) { setData(result); setSelected((result.roles ?? []).filter((role: Role) => role.flag).map((role: Role) => role.id)); } });
    return () => { cancelled = true; };
  }, [userId]);
  async function save() { setSaving(true); try { await userAuthRoleUpdate(userId, selected.map(Number)); navigate('/system/user'); } finally { setSaving(false); } }
  return <Page><Card title="分配角色"><Spin spinning={!data}>
    <Descriptions className="mb-5" items={[{ key: 'user', label: '用户账号', children: data?.user.userName }, { key: 'name', label: '用户昵称', children: data?.user.nickName }]} />
    <Table rowKey="id" dataSource={data?.roles ?? []} pagination={false} rowSelection={{ selectedRowKeys: selected, onChange: setSelected }} columns={[{ title: '角色名称', dataIndex: 'roleName' }, { title: '权限字符', dataIndex: 'roleKey' }, { title: '排序', dataIndex: 'roleSort' }]} />
    <Space className="mt-5"><Access code={['system:user:edit']}><Button type="primary" loading={saving} onClick={() => void save()}>保存</Button></Access><Button onClick={() => navigate('/system/user')}>返回</Button></Space>
  </Spin></Card></Page>;
}
