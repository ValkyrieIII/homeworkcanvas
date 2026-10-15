// pi 扩展：把抽到的作业候选经"工具参数"交出来。
// 用工具的 parameters schema 约束输出结构，比在提示词里要求"回一段 JSON"可靠。
import { writeFileSync } from "node:fs";
import { Type } from "typebox";

export default function (pi) {
  pi.registerTool({
    name: "submit_assignments",
    label: "提交作业候选",
    description:
      "把从课程材料里抽出来的作业候选提交给系统。读完材料后必须调用它，不要用文字复述结果。",
    promptSnippet: "Submit extracted assignments",
    promptGuidelines: [
      "读完材料后，用 submit_assignments 提交结果，不要用普通文字回答。",
    ],
    parameters: Type.Object({
      assignments: Type.Array(
        Type.Object({
          title: Type.String({ description: "作业标题" }),
          description: Type.Optional(Type.String({ description: "作业要求/说明" })),
          official_due_at: Type.Optional(
            Type.String({
              description:
                "RFC3339 截止时间，例如 2026-10-29T23:59:00+08:00；原文没写清就别填",
            }),
          ),
          raw_due_expression: Type.Optional(
            Type.String({ description: "原文里的截止时间写法" }),
          ),
          submission_method: Type.Optional(
            Type.String({ description: "提交方式或提交链接" }),
          ),
          course_instance_id: Type.Optional(
            Type.String({
              description: "课程实例 id，只能从给定清单里选；不确定就别填",
            }),
          ),
          evidence: Type.Optional(
            Type.Array(
              Type.Object({
                page_number: Type.Optional(Type.Number()),
                text_excerpt: Type.Optional(Type.String()),
                external_url: Type.Optional(Type.String()),
                original_name: Type.Optional(Type.String()),
              }),
            ),
          ),
          uncertain_fields: Type.Optional(
            Type.Array(Type.String({ description: "拿不准的字段名" })),
          ),
        }),
      ),
    }),
    async execute(_toolCallId, params) {
      const out = process.env.CANVAS_OUTPUT_FILE;
      if (!out) throw new Error("CANVAS_OUTPUT_FILE is not set");
      const items = Array.isArray(params?.assignments) ? params.assignments : [];
      writeFileSync(out, JSON.stringify(items), "utf8");
      return {
        content: [{ type: "text", text: `已提交 ${items.length} 条候选` }],
        terminate: true,
      };
    },
  });
}
