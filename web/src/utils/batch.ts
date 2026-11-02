/** Promise.allSettled 批量操作的汇总结果。 */
export interface BatchOutcome {
  succeeded: number;
  failed: number;
  firstError?: string;
}

/** 汇总 allSettled 结果：成功数 + 失败数 + 首个错误消息（用于逐项调用单条删除接口的批量操作）。 */
export function summarizeBatch(results: PromiseSettledResult<unknown>[]): BatchOutcome {
  const rejected = results.filter((r) => r.status === 'rejected');
  const first = rejected[0] as PromiseRejectedResult | undefined;
  return {
    succeeded: results.length - rejected.length,
    failed: rejected.length,
    firstError: first
      ? first.reason instanceof Error
        ? first.reason.message
        : String(first.reason)
      : undefined,
  };
}
