import path from "node:path";
import { BaseSequencer, type TestSpecification } from "vitest/node";
import { assignTestShards } from "./ci-test-shards.ts";

export class CITestSequencer extends BaseSequencer {
  override async shard(files: TestSpecification[]) {
    const { index, count } = this.ctx.config.shard!;
    const relative = (file: TestSpecification) => path.relative(this.ctx.config.root, file.moduleId).replace(/\\/g, "/");
    const assigned = new Set(assignTestShards(files.map(relative), count)[index - 1]);
    return files.filter(file => assigned.has(relative(file)));
  }
}
