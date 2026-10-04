/**
 * SYNTHETIC demo content for the fake core. Nothing here is a real policy, document or customer;
 * it exists so the Metro chat surface can be built and reviewed before M1/M2 deliver real data.
 */
import type { CitationLocator, DocumentPage } from "../../features/chat/contract.ts";

export const KB = { id: "kb-chinh-sach", name: "Chính sách nội bộ" } as const;

export const DOC_2026 = "doc-hoan-tien-2026";
export const DOC_2024 = "doc-hoan-tien-2024";

export const documentPages: DocumentPage[] = [
  {
    documentId: DOC_2026,
    fileName: "chinh-sach-hoan-tien-2026.pdf",
    kbName: KB.name,
    page: 3,
    pageCount: 12,
    section: "§2 Điều kiện hoàn tiền",
    version: "v2",
    blocks: [
      { id: "b0", kind: "heading", text: "Điều 2. Điều kiện hoàn tiền" },
      {
        id: "b1",
        kind: "paragraph",
        text: "2.1 Khách hàng có quyền yêu cầu hoàn tiền trong vòng 14 (mười bốn) ngày kể từ ngày nhận hàng, căn cứ theo thời điểm giao hàng thành công ghi nhận trên hệ thống vận chuyển.",
      },
      {
        id: "b3",
        kind: "paragraph",
        text: "2.2 Chứng từ hợp lệ gồm hoá đơn điện tử hoặc mã đơn hàng còn hiệu lực; không yêu cầu bản giấy.",
      },
      {
        id: "b4",
        kind: "paragraph",
        text: "2.3 Đơn hàng giao quốc tế chịu phí xử lý bằng 5% giá trị đơn, tối thiểu 50.000 đồng.",
      },
      {
        id: "b6",
        kind: "paragraph",
        text: "2.4 Thời gian xử lý hoàn tiền tối đa 7 ngày làm việc kể từ khi yêu cầu được chấp thuận.",
      },
      {
        id: "b5",
        kind: "paragraph",
        text: "2.5 Sản phẩm lỗi do nhà sản xuất được đổi mới hoặc hoàn tiền 100% không phụ thuộc thời hạn tại mục 2.1.",
      },
    ],
  },
  {
    documentId: DOC_2024,
    fileName: "chinh-sach-hoan-tien-2024.pdf",
    kbName: KB.name,
    page: 5,
    pageCount: 9,
    section: "§4 Hoàn tiền",
    version: "v1",
    blocks: [
      { id: "c0", kind: "heading", text: "Điều 4. Hoàn tiền" },
      {
        id: "c1",
        kind: "paragraph",
        text: "4.1 Khách hàng được yêu cầu hoàn tiền trong vòng 30 (ba mươi) ngày kể từ ngày mua, kèm hoá đơn giấy bản gốc do cửa hàng phát hành.",
      },
      {
        id: "c2",
        kind: "paragraph",
        text: "4.2 Đơn hàng quốc tế được xử lý như đơn trong nước, không phát sinh phí.",
      },
      { id: "c3", kind: "paragraph", text: "4.3 Tiền hoàn được chuyển về phương thức thanh toán ban đầu." },
    ],
  },
];

type Citation = {
  sourceId: string;
  documentId: string;
  fileName: string;
  title: string;
  locator: CitationLocator;
};

function citation(n: number, doc: DocumentPage, blockId: string, section: string, score: number): Citation {
  return {
    sourceId: String(n),
    documentId: doc.documentId,
    fileName: doc.fileName,
    title: section,
    locator: {
      documentId: doc.documentId,
      kbName: doc.kbName,
      page: doc.page,
      pageCount: doc.pageCount,
      section,
      blockIds: [blockId],
      score,
    },
  };
}

const [page2026, page2024] = documentPages as [DocumentPage, DocumentPage];

export const citations: Citation[] = [
  citation(1, page2026, "b1", "§2.1", 0.91),
  citation(2, page2024, "c1", "§4.1", 0.87),
  citation(3, page2026, "b3", "§2.2", 0.84),
  citation(4, page2026, "b4", "§2.3", 0.82),
  citation(5, page2026, "b5", "§2.5", 0.79),
];

export const refundScript = {
  question:
    "Chính sách hoàn tiền 2026 thay đổi gì so với bản 2024? Sau đó tạo issue trên Linear để đội CS cập nhật FAQ.",
  reasoningPlan:
    "Cần so sánh hai phiên bản chính sách, nên phải tìm cả bản 2024 lẫn 2026 trong KB Chính sách nội bộ trước khi trả lời.",
  kbQuery: "thời hạn, chứng từ, phí hoàn tiền 2024 vs 2026",
  readSection: { section: "§4 Hoàn tiền", fileName: page2024.fileName },
  reasoningCompose:
    "Đủ bằng chứng cho ba điều khoản thay đổi: thời hạn, chứng từ, phí quốc tế. Sẽ trình bày dạng bảng, rồi gọi Linear để tạo issue cho đội CS.",
  answer: [
    "Bản 2026 siết điều kiện hoàn tiền ở ba điểm: rút ngắn thời hạn, bỏ hoá đơn giấy và thêm phí xử lý cho đơn quốc tế.",
    "",
    "| Điều khoản | 2024 | 2026 |",
    "|---|---|---|",
    "| Thời hạn yêu cầu | 30 ngày [2] | **14 ngày** kể từ ngày nhận [1] |",
    "| Chứng từ | Hoá đơn giấy [2] | Hoá đơn điện tử / mã đơn [3] |",
    "| Phí quốc tế | Không có [2] | **5%**, tối thiểu 50.000đ [4] |",
    "",
    "Hàng lỗi do nhà sản xuất vẫn được đổi mới hoặc hoàn tiền 100%, không phụ thuộc thời hạn [5].",
  ].join("\n"),
  linear: {
    source: "Linear",
    title: "Tạo issue",
    input: {
      team: "CS-Support",
      title: "Cập nhật FAQ theo chính sách hoàn tiền 2026",
      labels: ["faq", "policy"],
    },
    output: { issue: "CS-214", url: "https://linear.app/example/issue/CS-214" },
  },
  afterApproval: "Đã tạo issue **CS-214** cho đội CS-Support để cập nhật FAQ.",
  afterDenial: "Mình đã bỏ qua bước tạo issue trên Linear theo lựa chọn của bạn.",
} as const;
