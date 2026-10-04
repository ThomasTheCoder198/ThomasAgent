import { ChatPage } from "@/features/chat/ChatPage";

export default async function ConversationPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ChatPage chatId={decodeURIComponent(id)} />;
}
