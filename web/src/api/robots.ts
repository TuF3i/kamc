import { apiDelete, apiGet, apiPost, downloadFile } from './http';
import type { CreateRobotResponse, Robot } from './types';

export interface CreateRobotInput {
  name: string;
  cluster_id: string;
}

export function listRobots(): Promise<Robot[]> {
  return apiGet<Robot[]>('/api/v1/robots');
}

export function createRobot(input: CreateRobotInput): Promise<CreateRobotResponse> {
  return apiPost<CreateRobotResponse>('/api/v1/robots', input);
}

export function deleteRobot(id: string): Promise<void> {
  return apiDelete<void>(`/api/v1/robots/${encodeURIComponent(id)}`);
}

export function downloadRobotKubeconfig(robot: Robot): Promise<void> {
  return downloadFile(
    `/api/v1/robots/${encodeURIComponent(robot.id)}/kubeconfig`,
    `kubeconfig-robot-${robot.name}.yaml`,
  );
}
