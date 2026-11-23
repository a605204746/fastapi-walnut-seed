import { useEffect, useRef, useState } from 'react';
import { Cascader, Select, TreeSelect } from 'antd';
function getPath(value: any, path?: string) {
  return path ? path.split('.').reduce((result, key) => result?.[key], value) : value;
}
/** Shared async option lifecycle, including stale-request protection and dependent params. */
function useOptions({
  api,
  params,
  resultField,
  labelField = 'label',
  valueField = 'value',
  childrenField = 'children',
  options,
}: any) {
  const [data, setData] = useState<any[]>(options ?? []);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const sequence = useRef(0);
  const encoded = JSON.stringify(params ?? {});
  useEffect(() => {
    if (!api) {
      setData(options ?? []);
      return;
    }
    const id = ++sequence.current;
    setLoading(true);
    setError(false);
    Promise.resolve()
      .then(() => api(params ?? {}))
      .then((result) => {
        if (id !== sequence.current) return;
        const rows = getPath(result, resultField) ?? [];
        if (!Array.isArray(rows)) throw new Error('选项接口必须返回数组');
        function map(list: any[]): any[] {
          return list.map((item) => ({
            ...item,
            label: item[labelField],
            value: item[valueField],
            children:
              Array.isArray(item[childrenField]) && item[childrenField].length
                ? map(item[childrenField])
                : undefined,
          }));
        }
        setData(map(rows));
      })
      .catch(() => {
        if (id === sequence.current) {
          setData([]);
          setError(true);
        }
      })
      .finally(() => {
        if (id === sequence.current) setLoading(false);
      });
    return () => {
      sequence.current++;
    };
  }, [api, encoded, resultField, labelField, valueField, childrenField, options]);
  return { data, loading, error };
}
function createApiComponent(kind: 'select' | 'tree' | 'cascader') {
  return function ApiComponent(props: any) {
    const { api, params, resultField, labelField, valueField, childrenField, options, ...rest } =
      props;
    const { data, loading, error } = useOptions({
      api,
      params,
      resultField,
      labelField,
      valueField,
      childrenField,
      options,
    });
    const common = {
      ...rest,
      loading,
      status: error ? ('error' as const) : rest.status,
      style: { width: '100%', ...rest.style },
    };
    return kind === 'tree' ? (
      <TreeSelect {...common} treeData={data} />
    ) : kind === 'cascader' ? (
      <Cascader {...common} options={data} />
    ) : (
      <Select {...common} options={data} />
    );
  };
}
export const ApiSelect = createApiComponent('select');
export const ApiTreeSelect = createApiComponent('tree');
export const ApiCascader = createApiComponent('cascader');
