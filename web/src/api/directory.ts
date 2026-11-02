import { apiGet, queryString } from './http';
import type { DirectoryOption } from './types';

export function listOrgs(query = ''): Promise<DirectoryOption[]> {
  return apiGet<DirectoryOption[]>(
    `/api/v1/directory/orgs${queryString({ q: query })}`,
  );
}

export function listUsers(query = ''): Promise<DirectoryOption[]> {
  return apiGet<DirectoryOption[]>(
    `/api/v1/directory/users${queryString({ q: query })}`,
  );
}
