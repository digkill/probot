import { Navigate, Route, Routes, Link, useNavigate } from 'react-router-dom'
import { getToken, getWorkspaceId, setToken, setWorkspaceId } from './api'
import { LangSwitch, useI18n } from './i18n'
import LoginPage from './pages/LoginPage'
import ResetPasswordPage from './pages/ResetPasswordPage'
import DashboardPage from './pages/DashboardPage'
import ContentPage from './pages/ContentPage'
import PlatformsPage from './pages/PlatformsPage'
import AgentsPage from './pages/AgentsPage'
import MentionsPage from './pages/MentionsPage'
import GraphPage from './pages/GraphPage'
import PlaybooksPage from './pages/PlaybooksPage'
import LinksPage from './pages/LinksPage'
import AnalyticsPage from './pages/AnalyticsPage'
import KarmaPage from './pages/KarmaPage'
import MailingLayout from './pages/mailing/MailingLayout'
import ContactsPage from './pages/mailing/ContactsPage'

function Shell({ children }: { children: React.ReactNode }) {
  const nav = useNavigate()
  const { t } = useI18n()
  const logout = () => {
    setToken('')
    setWorkspaceId('')
    nav('/login')
  }
  return (
    <div className="min-h-screen">
      <header className="border-b border-[var(--line)] px-6 py-4 flex items-center justify-between backdrop-blur">
        <div className="flex items-center gap-8">
          <Link to="/" className="text-xl tracking-tight font-semibold">
            <span className="text-[var(--accent)]">P</span>Robot
          </Link>
          <nav className="flex gap-4 text-sm text-[var(--muted)]">
            <Link to="/">{t('nav.overview')}</Link>
            <Link to="/content">{t('nav.content')}</Link>
            <Link to="/platforms">{t('nav.platforms')}</Link>
            <Link to="/agents">{t('nav.agents')}</Link>
            <Link to="/playbooks">{t('nav.playbooks')}</Link>
            <Link to="/links">{t('nav.links')}</Link>
            <Link to="/analytics">{t('nav.analytics')}</Link>
            <Link to="/karma">{t('nav.karma')}</Link>
            <Link to="/mentions">{t('nav.mentions')}</Link>
            <Link to="/graph">{t('nav.graph')}</Link>
            <Link to="/mailing/contacts">{t('nav.mailing')}</Link>
          </nav>
        </div>
        <div className="flex items-center gap-4">
          <LangSwitch />
          <button onClick={logout} className="text-sm text-[var(--muted)] hover:text-white">
            {t('nav.logout')}
          </button>
        </div>
      </header>
      <main className="px-6 py-8 max-w-6xl mx-auto">{children}</main>
    </div>
  )
}

function Private({ children }: { children: React.ReactNode }) {
  if (!getToken() || !getWorkspaceId()) return <Navigate to="/login" replace />
  return <Shell>{children}</Shell>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/reset-password" element={<ResetPasswordPage />} />
      <Route path="/" element={<Private><DashboardPage /></Private>} />
      <Route path="/content" element={<Private><ContentPage /></Private>} />
      <Route path="/platforms" element={<Private><PlatformsPage /></Private>} />
      <Route path="/agents" element={<Private><AgentsPage /></Private>} />
      <Route path="/playbooks" element={<Private><PlaybooksPage /></Private>} />
      <Route path="/links" element={<Private><LinksPage /></Private>} />
      <Route path="/analytics" element={<Private><AnalyticsPage /></Private>} />
      <Route path="/karma" element={<Private><KarmaPage /></Private>} />
      <Route path="/mentions" element={<Private><MentionsPage /></Private>} />
      <Route path="/graph" element={<Private><GraphPage /></Private>} />
      <Route path="/mailing" element={<Private><MailingLayout /></Private>}>
        <Route index element={<Navigate to="contacts" replace />} />
        <Route path="contacts" element={<ContactsPage />} />
      </Route>
    </Routes>
  )
}
