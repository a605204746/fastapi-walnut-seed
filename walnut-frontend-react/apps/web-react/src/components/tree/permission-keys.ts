export type PermissionKey = number | string;
export interface PermissionNode {
  id: PermissionKey;
  children?: PermissionNode[];
  menuType?: string;
}

export function allKeys(nodes: PermissionNode[]): PermissionKey[] {
  return nodes.flatMap((node) => [node.id, ...allKeys(node.children ?? [])]);
}

/** Include partially checked ancestors, as required by the role API. */
export function withAncestors(
  nodes: PermissionNode[],
  selected: PermissionKey[],
): PermissionKey[] {
  const keys = new Set(selected);
  function visit(node: PermissionNode): boolean {
    const children = (node.children ?? []).map(visit);
    const checked = keys.has(node.id) || children.some(Boolean);
    if (checked) keys.add(node.id);
    return checked;
  }
  nodes.forEach(visit);
  return [...keys];
}

export function togglePermission(
  nodes: PermissionNode[],
  selected: PermissionKey[],
  node: PermissionNode,
  checked: boolean,
  associated: boolean,
) {
  const keys = new Set(selected);
  const affected = associated
    ? allKeys([node])
    : [
        node.id,
        ...(node.children ?? [])
          .filter((n) => n.menuType === 'F')
          .map((n) => n.id),
      ];
  affected.forEach((key) => (checked ? keys.add(key) : keys.delete(key)));
  // Store explicit selections; ancestor completion is only performed at submission.
  return [...keys];
}
