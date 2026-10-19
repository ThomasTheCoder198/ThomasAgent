/** SYNTHETIC Agents for the fake core. Each org gets the same three, bound to that org's KBs by index. */
export type AgentSeed = {
  name: string;
  description: string;
  instructions: string;
  kbIndex: number;
  hoursAgo: number;
};

export const agentSeeds: AgentSeed[] = [
  {
    name: "Trợ lý pháp chế",
    description: "Trả lời câu hỏi về chính sách và hợp đồng.",
    instructions: "Trả lời ngắn gọn, luôn trích dẫn.",
    kbIndex: 0,
    hoursAgo: 3,
  },
  {
    name: "CSKH đơn hàng",
    description: "Hỗ trợ khách về đơn hàng và đổi trả.",
    instructions: "",
    kbIndex: 1,
    hoursAgo: 20,
  },
  { name: "Kế toán", description: "", instructions: "", kbIndex: 2, hoursAgo: 52 },
];
