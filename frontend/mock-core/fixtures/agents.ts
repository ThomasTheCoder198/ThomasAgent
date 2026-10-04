/** SYNTHETIC Agents, Knowledge Bases and tool sources for the fake core. Nothing here is real. */
import type { AgentDetail, AgentToolSource, KnowledgeBaseSummary } from "../../features/agents/contract.ts";

const HOUR_MS = 3_600_000;
const ago = (hours: number) => new Date(Date.now() - hours * HOUR_MS).toISOString();

export const knowledgeBases: KnowledgeBaseSummary[] = [
  { id: "kb-chinh-sach", name: "Chính sách nội bộ", documents: 42, updatedAt: ago(3) },
  { id: "kb-faq-don-hang", name: "FAQ đơn hàng", documents: 118, updatedAt: ago(20) },
  { id: "kb-chung-tu", name: "Hoá đơn & chứng từ", documents: 1_204, updatedAt: ago(52) },
  { id: "kb-hop-dong", name: "Hợp đồng nhà cung cấp", documents: 37, updatedAt: ago(96) },
];

/** KBs owned by another org (tenant). Never listed to this operator; patching an Agent onto one is forbidden. */
export const foreignKnowledgeBases: KnowledgeBaseSummary[] = [
  { id: "kb-org-khac", name: "Tài liệu của org khác", documents: 12, updatedAt: ago(10) },
];

const linear = (): AgentToolSource => ({
  id: "src-linear",
  line: "mcp",
  kind: "mcp",
  name: "Linear",
  tools: [
    {
      id: "linear.create_issue",
      name: "create_issue",
      title: "Tạo issue",
      description: "Tạo issue mới trong một team.",
      risky: true,
      policy: "ask",
    },
    {
      id: "linear.search_issues",
      name: "search_issues",
      title: "Tìm issue",
      description: "Tìm issue theo từ khoá và trạng thái.",
      risky: false,
      policy: "auto",
    },
  ],
});

const gmail = (): AgentToolSource => ({
  id: "src-gmail",
  line: "cmp",
  kind: "composio",
  name: "Gmail",
  tools: [
    {
      id: "gmail.send_email",
      name: "GMAIL_SEND_EMAIL",
      title: "Gửi email",
      description: "Gửi email từ tài khoản đã kết nối.",
      risky: true,
      policy: "ask",
    },
    {
      id: "gmail.search",
      name: "GMAIL_FETCH_EMAILS",
      title: "Tìm email",
      description: "Đọc email theo bộ lọc.",
      risky: false,
      policy: "auto",
    },
  ],
});

const invoices = (): AgentToolSource => ({
  id: "src-hoa-don",
  line: "app",
  kind: "app",
  name: "Hoá đơn",
  tools: [
    {
      id: "hoadon.lookup",
      name: "lookup_invoice",
      title: "Tra hoá đơn",
      description: "Tra cứu hoá đơn theo mã đơn hàng.",
      risky: false,
      policy: "auto",
    },
    {
      id: "hoadon.void",
      name: "void_invoice",
      title: "Huỷ hoá đơn",
      description: "Huỷ một hoá đơn đã phát hành.",
      risky: true,
      policy: "off",
    },
  ],
});

export const agents: AgentDetail[] = [
  {
    id: "agent-phap-che",
    name: "Trợ lý pháp chế",
    description: "Trả lời câu hỏi về chính sách, so sánh phiên bản và mở việc cho đội liên quan.",
    instructions:
      "Bạn là trợ lý pháp chế của công ty. Chỉ trả lời dựa trên Knowledge Base được gắn và luôn trích dẫn nguồn.\nKhi không tìm thấy thông tin phù hợp, nói rõ là không tìm thấy thay vì đoán.\nTrình bày so sánh dạng bảng khi có từ hai phiên bản trở lên.",
    kbId: "kb-chinh-sach",
    toolSources: [linear(), gmail(), invoices()],
    modelOverrides: {},
    contextFiles: [{ id: "cf-1", name: "quy-trinh-duyet-hop-dong.md", sizeBytes: 8_412 }],
    updatedAt: ago(2),
  },
  {
    id: "agent-cskh",
    name: "CSKH đơn hàng",
    description: "Hỗ trợ khách về đổi trả, tình trạng đơn và hoá đơn.",
    instructions:
      "Bạn hỗ trợ khách hàng về đơn hàng. Giọng thân thiện, ngắn gọn. Không hứa hoàn tiền khi chính sách không cho phép.",
    kbId: "kb-faq-don-hang",
    toolSources: [gmail(), invoices()],
    modelOverrides: { "chat.default": "fast" },
    contextFiles: [],
    updatedAt: ago(30),
  },
  {
    id: "agent-ke-toan",
    name: "Kế toán nội bộ",
    description: "Đối chiếu hoá đơn, chứng từ và trả lời câu hỏi kế toán.",
    instructions: "Bạn là trợ lý kế toán. Luôn ghi số tiền theo định dạng Việt Nam và nêu rõ kỳ kế toán.",
    kbId: "kb-chung-tu",
    toolSources: [invoices()],
    modelOverrides: {},
    contextFiles: [],
    updatedAt: ago(75),
  },
  {
    // Bound to a KB this operator cannot see (imported config): exercises the "unknown Knowledge Base" state.
    id: "agent-nhap-khau",
    name: "Agent nhập từ cấu hình cũ",
    description: "Gắn với một Knowledge Base không thuộc org này.",
    instructions: "Trả lời ngắn gọn.",
    kbId: "kb-org-khac",
    toolSources: [],
    modelOverrides: {},
    contextFiles: [],
    updatedAt: ago(120),
  },
];
