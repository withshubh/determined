import React from 'react';
import { Helmet } from 'react-helmet';

import PageHeader from 'components/PageHeader';
import { CommonProps } from 'types';

import Info from '../contexts/Info';

import css from './Page.module.scss';

export interface BreadCrumbRoute {
  breadcrumbName: string;
  path: string;
}

export interface Props extends CommonProps {
  breadcrumb?: BreadCrumbRoute[];
  docTitle?: string;
  id?: string;
  options?: React.ReactNode;
  stickHeader?: boolean;
  subTitle?: React.ReactNode;
  title?: string;
}

const getFullDocTitle = (title?: string, clusterName?: string) => {
  const segmentList = [ 'Determined' ];

  if (clusterName) segmentList.unshift(clusterName);
  if (title) segmentList.unshift(title);

  return segmentList.join(' - ');
};

const Page: React.FC<Props> = (props: Props) => {
  const classes = [ props.className, css.base ];
  const info = Info.useStateContext();
  const showHeader = props.breadcrumb || props.title;

  const docTitle = getFullDocTitle(
    props.docTitle || props.title,
    info.clusterName,
  );

  if (props.stickHeader) classes.push(css.stickyHeader);

  return (
    <main className={classes.join(' ')} id={props.id}>
      <Helmet>
        <title>{docTitle}</title>
      </Helmet>
      {showHeader && <PageHeader
        breadcrumb={props.breadcrumb}
        options={props.options}
        sticky={props.stickHeader}
        subTitle={props.subTitle}
        title={props.title} />}
      <div className={css.body}>{props.children}</div>
    </main>
  );
};

export default Page;
