package com.lingxi.ai.orchestrator.repository;

import com.lingxi.ai.orchestrator.entity.NodeDependency;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

import java.util.List;

@Repository
public interface NodeDependencyRepository extends JpaRepository<NodeDependency, NodeDependency.NodeDependencyId> {

    List<NodeDependency> findByChildNodeId(String childNodeId);

    List<NodeDependency> findByParentNodeId(String parentNodeId);
}
