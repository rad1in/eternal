import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Layout,
  Result,
  Row,
  Spin,
  Statistic,
  message,
} from 'antd';
import {
  ApiOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useTheme } from '@/hooks/useTheme';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { useTunnelList, useTunnelMutations, useTunnelOptions } from '@/api/queries/useTunnels';
import type { TunnelView } from '@/generated/zod';
import { buildCreateRequest, tunnelCounts } from '@/lib/tunnel';
import { setMessageInstance } from '@/utils/messageBus';
import TunnelDrawer from './TunnelDrawer';
import TunnelList from './TunnelList';
import TunnelWizard from './TunnelWizard';
import './Tunnels.css';

export default function TunnelsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const { tunnels, loading, fetched, error, refetch } = useTunnelList();
  const { nodes } = useNodesQuery();
  const options = useTunnelOptions();
  const { create, remove, restart } = useTunnelMutations();

  const [wizardOpen, setWizardOpen] = useState(false);
  const [openPair, setOpenPair] = useState<string | null>(null);
  const counts = useMemo(() => tunnelCounts(tunnels), [tunnels]);

  const pageClass = ['tunnels-page', isDark && 'is-dark', isUltra && 'is-ultra']
    .filter(Boolean)
    .join(' ');

  const onDelete = async (row: TunnelView) => {
    if (await remove.mutateAsync({ pair: row.pairUuid, force: false })) {
      messageApi.success(t('tunnel.deleted'));
    }
  };
  const onRestart = (row: TunnelView) =>
    restart.mutate(row.pairUuid, {
      onSuccess: (ok) => ok && messageApi.success(t('tunnel.restarted')),
    });

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : error ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={error}
                  extra={
                    <Button type="primary" onClick={() => refetch()}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : (
                <Row gutter={[isMobile ? 8 : 16, isMobile ? 8 : 12]}>
                  {options && !options.binaryAvailable && (
                    <Col span={24}>
                      <Alert type="warning" showIcon title={t('tunnel.noBinary')} />
                    </Col>
                  )}
                  <Col span={24}>
                    <Card size="small" hoverable className="summary-card">
                      <Row gutter={[16, 12]}>
                        <Col xs={8}>
                          <Statistic
                            title={t('tunnel.title')}
                            value={counts.total}
                            prefix={<ApiOutlined />}
                          />
                        </Col>
                        <Col xs={8}>
                          <Statistic
                            title={t('tunnel.state.up')}
                            value={counts.up}
                            prefix={
                              <CheckCircleOutlined style={{ color: 'var(--ant-color-success)' }} />
                            }
                          />
                        </Col>
                        <Col xs={8}>
                          <Statistic
                            title={t('tunnel.state.down')}
                            value={counts.down}
                            prefix={
                              <CloseCircleOutlined style={{ color: 'var(--ant-color-error)' }} />
                            }
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>
                  <Col span={24}>
                    <Card size="small">
                      <div className="toolbar">
                        <Button
                          type="primary"
                          icon={<PlusOutlined />}
                          onClick={() => setWizardOpen(true)}
                        >
                          {t('tunnel.add')}
                        </Button>
                        <Button
                          icon={<ReloadOutlined />}
                          loading={loading}
                          onClick={() => refetch()}
                        >
                          {t('refresh')}
                        </Button>
                      </div>
                      <TunnelList
                        tunnels={tunnels}
                        loading={loading}
                        onOpen={(row) => setOpenPair(row.pairUuid)}
                        onRestart={onRestart}
                        onDelete={onDelete}
                      />
                    </Card>
                  </Col>
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <TunnelWizard
          open={wizardOpen}
          nodes={nodes}
          capabilities={options}
          creating={create.isPending}
          onCreate={(values) => create.mutateAsync(buildCreateRequest(values))}
          onClose={() => setWizardOpen(false)}
        />
        <TunnelDrawer
          pair={openPair}
          canSeeToken={options?.canSeeToken ?? false}
          onClose={() => setOpenPair(null)}
        />
      </Layout>
    </ConfigProvider>
  );
}
